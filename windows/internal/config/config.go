package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const SchemaVersion = 3
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
	Profiles       []VoiceProfile `json:"profiles"`
	HoldKey        string         `json:"hold_key"`
	ToggleStartKey string         `json:"toggle_start_key"`
	ToggleStopKey  string         `json:"toggle_stop_key"`
	StopDelayMS    int            `json:"stop_delay_ms"`
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
		Shortcuts: []Shortcut{{"音量-", "VolumeDown", true}, {"上", "Up", true}, {"音量+", "VolumeUp", true}, {"退格", "Backspace", true},
			{"左", "Left", true}, {"下", "Down", true}, {"右", "Right", true}, {"回车", "Return", true}},
		Voice: Voice{Profiles: DefaultVoiceProfiles(), HoldKey: "RightAlt", ToggleStartKey: "RightCtrl+L", ToggleStopKey: "RightCtrl+L", StopDelayMS: 200}, Gain: 1, Sensitivity: PointerBaseSensitivity, NaturalScroll: true, KeyboardBackend: "auto"}
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
	// Missing profile data must not silently inherit new-install defaults.
	c.Voice = Voice{StopDelayMS: 200}
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
		c.Voice = c.Voice.Normalized()
		return c, c.Validate()
	}
	if old.SchemaVersion < 0 || old.SchemaVersion > SchemaVersion {
		return c, fmt.Errorf("不支持配置版本 %d", old.SchemaVersion)
	}
	if old.SchemaVersion < 2 {
		if len(c.Shortcuts) != 4 {
			return c, fmt.Errorf("旧配置必须包含四个快捷键")
		}
		for i := range c.Shortcuts {
			c.Shortcuts[i].Enabled = true
		}
		// New-install defaults must not enable extra shortcuts during an upgrade.
		for len(c.Shortcuts) < ShortcutCount {
			c.Shortcuts = append(c.Shortcuts, Shortcut{Label: fmt.Sprintf("快捷键 %d", len(c.Shortcuts)+1)})
		}
		switch old.Voice.Mode {
		case "", "mic":
		case "hold":
			c.Voice.HoldKey = old.Voice.Start
		case "toggle":
			c.Voice.ToggleStartKey, c.Voice.ToggleStopKey = old.Voice.Start, old.Voice.Stop
		default:
			return c, fmt.Errorf("无效旧语音模式")
		}
	}
	c.Voice.Profiles = DefaultVoiceProfiles()
	c.Voice.Profiles[0].ToggleStartKey, c.Voice.Profiles[0].ToggleStopKey = c.Voice.ToggleStartKey, c.Voice.ToggleStopKey
	c.Voice.Profiles[1].HoldKey = c.Voice.HoldKey
	c.Voice = c.Voice.Normalized()
	c.SchemaVersion = SchemaVersion
	if e = c.Validate(); e != nil {
		return c, e
	}
	backupVersion := old.SchemaVersion
	if backupVersion < 1 {
		backupVersion = 1
	}
	if e = backupOriginal(dir, backupVersion, b); e != nil {
		return c, e
	}
	return c, Save(dir, c)
}

func backupOriginal(dir string, version int, b []byte) error {
	for suffix := 0; ; suffix++ {
		name := fmt.Sprintf("config.v%d.bak", version)
		if suffix > 0 {
			name += fmt.Sprintf(".%d", suffix)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			old, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if bytes.Equal(old, b) {
				return nil
			}
			continue // Keep any earlier backup and retain this original as well.
		}
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
		}
		return err
	}
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
	return c.Voice.Validate()
}
func Save(dir string, c Config) error {
	c.Sensitivity = PointerBaseSensitivity
	c.Voice = c.Voice.Normalized()
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
