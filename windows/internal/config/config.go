package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const SchemaVersion = 2
const ShortcutCount = 8

// The former PC sensitivity of 2 is now the Android device's 1x baseline.
// Keep the v2 field fixed for clients that still read the PC configuration.
const PointerBaseSensitivity = 2.0

type Shortcut struct {
	Label   string `json:"label"`
	Chord   string `json:"chord"`
	Enabled bool   `json:"enabled"`
}
type Voice struct {
	HoldKey        string `json:"hold_key"`
	ToggleStartKey string `json:"toggle_start_key"`
	ToggleStopKey  string `json:"toggle_stop_key"`
	StopDelayMS    int    `json:"stop_delay_ms"`
}
type Config struct {
	SchemaVersion   int        `json:"schema_version"`
	Revision        uint64     `json:"revision"`
	HTTPPort        int        `json:"http_port"`
	WSSPort         int        `json:"wss_port"`
	UDPPort         int        `json:"udp_port"`
	Shortcuts       []Shortcut `json:"shortcuts"`
	Voice           Voice      `json:"voice"`
	AudioDevice     string     `json:"audio_device"`
	Gain            float64    `json:"gain"`
	Sensitivity     float64    `json:"sensitivity"`
	NaturalScroll   bool       `json:"natural_scroll"`
	KeyboardBackend string     `json:"keyboard_backend,omitempty"`
}

func Default() Config {
	return Config{SchemaVersion: SchemaVersion, Revision: 1, HTTPPort: 41080, WSSPort: 41443, UDPPort: 41444,
		Shortcuts: []Shortcut{{"复制", "Ctrl+C", true}, {"粘贴", "Ctrl+V", true}, {"撤销", "Ctrl+Z", true}, {"回车", "Enter", true},
			{"快捷键 5", "", false}, {"快捷键 6", "", false}, {"快捷键 7", "", false}, {"快捷键 8", "", false}},
		Voice: Voice{StopDelayMS: 200}, Gain: 1, Sensitivity: PointerBaseSensitivity, NaturalScroll: true, KeyboardBackend: "auto"}
}
func Directory() string { return filepath.Join(os.Getenv("LOCALAPPDATA"), "TapDeck") }
func Load(dir string) (Config, error) {
	c := Default()
	b, e := os.ReadFile(filepath.Join(dir, "config.json"))
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, e
	}
	c.Sensitivity = PointerBaseSensitivity
	var old struct {
		SchemaVersion int                                `json:"schema_version"`
		Voice         struct{ Mode, Start, Stop string } `json:"voice"`
	}
	if e = json.Unmarshal(b, &old); e != nil {
		return c, e
	}
	if old.SchemaVersion == SchemaVersion {
		return c, c.Validate()
	}
	if old.SchemaVersion < 0 || old.SchemaVersion > SchemaVersion {
		return c, fmt.Errorf("不支持配置版本 %d", old.SchemaVersion)
	}
	if len(c.Shortcuts) != 4 {
		return c, fmt.Errorf("旧配置必须包含四个快捷键")
	}
	for i := range c.Shortcuts {
		c.Shortcuts[i].Enabled = true
	}
	c.Shortcuts = append(c.Shortcuts, Default().Shortcuts[4:]...)
	switch old.Voice.Mode {
	case "", "mic":
	case "hold":
		c.Voice.HoldKey = old.Voice.Start
	case "toggle":
		c.Voice.ToggleStartKey, c.Voice.ToggleStopKey = old.Voice.Start, old.Voice.Stop
	default:
		return c, fmt.Errorf("无效旧语音模式")
	}
	c.SchemaVersion = SchemaVersion
	if e = c.Validate(); e != nil {
		return c, e
	}
	backup, e := os.OpenFile(filepath.Join(dir, "config.v1.bak"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil && !os.IsExist(e) {
		return c, e
	}
	if e == nil {
		_, e = backup.Write(b)
		closeErr := backup.Close()
		if e != nil {
			return c, e
		}
		if closeErr != nil {
			return c, closeErr
		}
	}
	return c, Save(dir, c)
}
func (c Config) Validate() error {
	if c.KeyboardBackend != "" && c.KeyboardBackend != "auto" && c.KeyboardBackend != "hid" && c.KeyboardBackend != "sendinput" {
		return fmt.Errorf("无效键盘发送方式")
	}
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("不支持配置版本 %d", c.SchemaVersion)
	}
	if len(c.Shortcuts) != ShortcutCount {
		return fmt.Errorf("必须配置八个快捷键槽位")
	}
	enabled := 0
	for i, k := range c.Shortcuts {
		if k.Enabled {
			enabled++
			if strings.TrimSpace(k.Label) == "" || strings.TrimSpace(k.Chord) == "" {
				return fmt.Errorf("快捷键 %d 需要名称和按键", i+1)
			}
		}
	}
	if enabled == 0 {
		return fmt.Errorf("请至少启用一个快捷键")
	}
	for _, p := range []int{c.HTTPPort, c.WSSPort, c.UDPPort} {
		if p < 1024 || p > 65535 {
			return fmt.Errorf("端口必须在 1024–65535")
		}
	}
	if c.HTTPPort == c.WSSPort {
		return fmt.Errorf("HTTP 和 WSS 端口不能相同")
	}
	if c.Gain < 0 || c.Gain > 3 || c.Sensitivity < 0.1 || c.Sensitivity > 5 {
		return fmt.Errorf("音量或灵敏度超出范围")
	}
	if c.Voice.StopDelayMS < 0 || c.Voice.StopDelayMS > 1000 {
		return fmt.Errorf("结束延迟必须在 0–1000 ms")
	}
	return nil
}
func Save(dir string, c Config) error {
	c.Sensitivity = PointerBaseSensitivity
	if e := c.Validate(); e != nil {
		return e
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	return AtomicWrite(filepath.Join(dir, "config.json"), b)
}
func AtomicWrite(path string, b []byte) error {
	tmp := path + ".tmp"
	if e := os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
