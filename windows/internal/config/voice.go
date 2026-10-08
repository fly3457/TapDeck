package config

import (
	"fmt"
	"strings"
)

const VoiceProfileCount = 3

type VoiceProfile struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	Mode           string `json:"mode"`
	HoldKey        string `json:"hold_key"`
	ToggleStartKey string `json:"toggle_start_key"`
	ToggleStopKey  string `json:"toggle_stop_key"`
}

func DefaultVoiceProfiles() []VoiceProfile {
	return []VoiceProfile{
		{ID: "voice-1", Name: "单击语音输入", Enabled: true, Mode: "toggle", ToggleStartKey: "RightCtrl+L", ToggleStopKey: "RightCtrl+L"},
		{ID: "voice-2", Name: "长按语音输入", Enabled: true, Mode: "hold", HoldKey: "RightAlt"},
		{ID: "voice-3", Name: "自定义语音输入", Mode: "hold"},
	}
}

func VoiceNameLength(name string) int {
	n := 0
	for _, r := range strings.TrimSpace(name) {
		if r <= 127 {
			n++
		} else {
			n += 2
		}
	}
	return n
}

func (v Voice) Validate() error {
	if len(v.Profiles) != VoiceProfileCount {
		return fmt.Errorf("必须配置三组语音")
	}
	for i, p := range v.Profiles {
		if p.ID != fmt.Sprintf("voice-%d", i+1) {
			return fmt.Errorf("语音配置 %d 的 ID 或顺序无效", i+1)
		}
		if strings.TrimSpace(p.Name) == "" || VoiceNameLength(p.Name) > 16 {
			return fmt.Errorf("语音配置 %d 的名称不能为空，最多 8 个汉字或 16 个英文字符（可混排）", i+1)
		}
		if p.Mode != "hold" && p.Mode != "toggle" {
			return fmt.Errorf("语音配置 %d 的类型无效", i+1)
		}
	}
	return nil
}

func (v Voice) FirstEnabled(mode string) (VoiceProfile, bool) {
	for _, p := range v.Profiles {
		if p.Enabled && p.Mode == mode {
			return p, true
		}
	}
	return VoiceProfile{}, false
}

func (p VoiceProfile) Keys() []string {
	if p.Mode == "hold" {
		return []string{p.HoldKey}
	}
	return []string{p.ToggleStartKey, p.ToggleStopKey}
}

// Legacy fields are a projection for v2 clients, never a second source of truth.
// Clone before trimming so a caller cannot mutate a running recording/config snapshot.
func (v Voice) Normalized() Voice {
	v.Profiles = append([]VoiceProfile(nil), v.Profiles...)
	for i := range v.Profiles {
		p := &v.Profiles[i]
		p.Name = strings.TrimSpace(p.Name)
		p.HoldKey = strings.TrimSpace(p.HoldKey)
		p.ToggleStartKey = strings.TrimSpace(p.ToggleStartKey)
		p.ToggleStopKey = strings.TrimSpace(p.ToggleStopKey)
	}
	v.HoldKey, v.ToggleStartKey, v.ToggleStopKey = "", "", ""
	if p, ok := v.FirstEnabled("hold"); ok {
		v.HoldKey = p.HoldKey
	}
	if p, ok := v.FirstEnabled("toggle"); ok {
		v.ToggleStartKey, v.ToggleStopKey = p.ToggleStartKey, p.ToggleStopKey
	}
	return v
}
