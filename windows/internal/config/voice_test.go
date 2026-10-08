package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVoiceDefaultsAndNameBoundaries(t *testing.T) {
	c := Default()
	p := c.Voice.Profiles
	if len(p) != 3 || p[0].Mode != "toggle" || p[0].ToggleStartKey != "RightCtrl+L" || p[0].ToggleStopKey != "RightCtrl+L" || !p[0].Enabled || p[1].HoldKey != "RightAlt" || !p[1].Enabled || p[2].HoldKey != "Ctrl+Shift+M" || p[2].Enabled {
		t.Fatal(p)
	}
	for _, tc := range []struct {
		name  string
		valid bool
	}{
		{"  中文English12345  ", true}, {"中文English123456", false},
		{strings.Repeat("中", 8), true}, {strings.Repeat("中", 9), false},
		{strings.Repeat("a", 16), true}, {strings.Repeat("a", 17), false},
		{strings.Repeat("😀", 8), true}, {strings.Repeat("😀", 9), false}, {" \t\n", false},
	} {
		c.Voice.Profiles[0].Name = tc.name
		if (c.Validate() == nil) != tc.valid {
			t.Errorf("name %q", tc.name)
		}
	}
	c = Default()
	for i := range c.Voice.Profiles {
		c.Voice.Profiles[i].Enabled = false
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Voice.Profiles[1].ID = "voice-1"
	if c.Validate() == nil {
		t.Fatal("duplicate/out of order ID accepted")
	}
}

func TestV2VoiceMigrationPreservesKeysSettingsAndOriginal(t *testing.T) {
	for _, key := range []string{"", "F9"} {
		dir := t.TempDir()
		c := Default()
		c.SchemaVersion = 2
		c.Revision = 25
		c.Gain = 1.7
		c.AudioDevice = "custom audio"
		c.Shortcuts[2].Enabled = false
		c.Voice = Voice{HoldKey: key, ToggleStartKey: "RightAlt", ToggleStopKey: "F10", StopDelayMS: 321}
		b, _ := json.Marshal(c)
		path := filepath.Join(dir, "config.json")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got.SchemaVersion != 3 || got.Revision != 25 || got.Gain != 1.7 || got.AudioDevice != c.AudioDevice || got.Shortcuts[2].Enabled || got.Voice.StopDelayMS != 321 {
			t.Fatal(got)
		}
		p := got.Voice.Profiles
		if p[0].ToggleStartKey != "RightAlt" || p[0].ToggleStopKey != "F10" || p[1].HoldKey != key || p[2].Enabled || p[2].HoldKey != "Ctrl+Shift+M" {
			t.Fatal(p)
		}
		backup, err := os.ReadFile(filepath.Join(dir, "config.v2.bak"))
		if err != nil || string(backup) != string(b) {
			t.Fatal("backup mismatch", err)
		}
		got.Voice.Profiles[0].Name = "  更名  "
		if err := Save(dir, got); err != nil {
			t.Fatal(err)
		}
		got, err = Load(dir)
		if err != nil || got.Voice.Profiles[0].Name != "更名" {
			t.Fatal(got, err)
		}
	}
}

func TestVoiceFailedSaveOrMigrationLeavesOriginal(t *testing.T) {
	for _, kind := range []string{"name", "write", "migration", "missing"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			c := Default()
			if kind == "migration" {
				c.SchemaVersion = 2
				c.Shortcuts = nil
			}
			if kind == "missing" {
				c.Voice.Profiles = nil
			}
			b, _ := json.Marshal(c)
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, b, 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "name":
				c.Voice.Profiles[1].Name = ""
				err = Save(dir, c)
			case "write":
				if e := os.Mkdir(path+".tmp", 0700); e != nil {
					t.Fatal(e)
				}
				c.Voice.Profiles[1].Name = "改名"
				err = Save(dir, c)
			default:
				_, err = Load(dir)
			}
			if err == nil {
				t.Fatal("expected failure")
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(b) {
				t.Fatal("original overwritten")
			}
		})
	}
}

func TestMigrationBackupFailureAndExistingBackup(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		dir := t.TempDir()
		c := Default()
		c.SchemaVersion = 2
		b, _ := json.Marshal(c)
		path := filepath.Join(dir, "config.json")
		backup := filepath.Join(dir, "config.v2.bak")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
		if blocked {
			if err := os.Mkdir(backup, 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir); err == nil {
				t.Fatal("backup failure ignored")
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(b) {
				t.Fatal("failed migration changed original")
			}
		} else {
			if err := os.WriteFile(backup, []byte("older original"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir); err != nil {
				t.Fatal(err)
			}
			old, _ := os.ReadFile(backup)
			next, _ := os.ReadFile(backup + ".1")
			if string(old) != "older original" || string(next) != string(b) {
				t.Fatal("original backups lost")
			}
		}
	}
}
