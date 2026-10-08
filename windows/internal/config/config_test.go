package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPointerBaselineNormalizesOldConfigAndSavedConfig(t *testing.T) {
	for _, old := range []float64{0.1, 1.5, 2, 5} {
		t.Run(fmt.Sprint(old), func(t *testing.T) {
			dir := t.TempDir()
			c := Default()
			if c.Sensitivity != PointerBaseSensitivity {
				t.Fatal("incorrect default baseline")
			}
			c.Sensitivity = old
			c.Shortcuts[0].Label = "自定义复制"
			b, _ := json.Marshal(c)
			if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(dir)
			if err != nil || loaded.Sensitivity != 2 || loaded.Shortcuts[0].Label != c.Shortcuts[0].Label {
				t.Fatalf("load: %+v, %v", loaded, err)
			}
			if err = Save(dir, c); err != nil {
				t.Fatal(err)
			}
			b, err = os.ReadFile(filepath.Join(dir, "config.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(b, &c); err != nil {
				t.Fatal(err)
			}
			if c.Sensitivity != 2 || c.Shortcuts[0].Label != "自定义复制" {
				t.Fatal("saved baseline or bindings incorrect")
			}
		})
	}
}

func TestLegacyMigrationPreservesBindingsAndBackup(t *testing.T) {
	for _, mode := range []string{"mic", "hold", "toggle"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			original := []byte(fmt.Sprintf(`{"revision":7,"http_port":41080,"wss_port":41443,"udp_port":41444,"shortcuts":[{"label":"复制","chord":"Ctrl+C"},{"label":"粘贴","chord":"Ctrl+V"},{"label":"撤销","chord":"Ctrl+Z"},{"label":"回车","chord":"Enter"}],"voice":{"mode":%q,"start":"RightAlt","stop":"F9","stop_delay_ms":135},"sensitivity":2,"gain":1,"natural_scroll":true}`, mode))
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			c, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if c.SchemaVersion != 3 || c.Revision != 7 || len(c.Shortcuts) != 8 || c.Sensitivity != 2 || c.Voice.StopDelayMS != 135 {
				t.Fatalf("migration lost settings: %+v", c)
			}
			for i, k := range c.Shortcuts {
				if k.Enabled != (i < 4) {
					t.Fatal("wrong enabled slots")
				}
			}
			if (c.Voice.HoldKey == "RightAlt") != (mode == "hold") || (c.Voice.ToggleStartKey == "RightAlt") != (mode == "toggle") || (c.Voice.ToggleStopKey == "F9") != (mode == "toggle") {
				t.Fatalf("wrong voice migration: %+v", c.Voice)
			}
			backup, err := os.ReadFile(filepath.Join(dir, "config.v1.bak"))
			if err != nil || string(backup) != string(original) {
				t.Fatal("missing exact legacy backup", err)
			}
			c.Shortcuts[1].Enabled = false
			if err := Save(dir, c); err != nil {
				t.Fatal(err)
			}
			reloaded, err := Load(dir)
			if err != nil || reloaded.Shortcuts[1].Enabled {
				t.Fatal("v2 checkbox was reset", err)
			}
			again, _ := os.ReadFile(filepath.Join(dir, "config.v1.bak"))
			if string(again) != string(original) {
				t.Fatal("backup overwritten")
			}
		})
	}
}

func TestEnabledShortcutValidation(t *testing.T) {
	c := Default()
	for i := range c.Shortcuts {
		c.Shortcuts[i].Enabled = false
	}
	if c.Validate() == nil {
		t.Fatal("zero shortcuts allowed")
	}
	c.Shortcuts[7].Enabled = true
	if c.Validate() == nil {
		t.Fatal("unconfigured enabled shortcut allowed")
	}
	c.Shortcuts[7].Chord = "RightCtrl+F9"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
