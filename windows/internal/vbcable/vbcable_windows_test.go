//go:build windows

package vbcable

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"tapdeck/internal/audio"
	"testing"
)

func TestDetectStates(t *testing.T) {
	in := []audio.Device{{ID: "render", Name: "CABLE Input (VB-Audio Virtual Cable)"}}
	out := []audio.Device{{ID: "capture", Name: "CABLE Output (VB-Audio Virtual Cable)"}}
	for _, tt := range []struct {
		name            string
		driver          bool
		render, capture []audio.Device
		want            State
	}{
		{"missing", false, nil, nil, Missing},
		{"installed without endpoints", true, nil, nil, Unavailable},
		{"disabled capture", true, in, nil, Unavailable},
		{"endpoint evidence", false, in, nil, Unavailable},
		{"ready", true, in, out, Ready},
		{"renamed or incomplete endpoints", true, []audio.Device{{ID: "custom", Name: "My microphone"}}, out, Unavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := classify(tt.driver, tt.render, tt.capture); got.State != tt.want {
				t.Fatal(got)
			}
		})
	}
}

func TestExtractPreservesEntireOriginalArchiveAndValidSignatures(t *testing.T) {
	dir := t.TempDir()
	setup, err := Extract(dir)
	if err != nil {
		t.Fatal(err)
	}
	if setup != filepath.Join(dir, "VBCABLE_Setup_x64.exe") {
		t.Fatal(setup)
	}
	b, _ := assets.ReadFile("assets/" + Filename)
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		want, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Name)))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("original file changed: %s (%v)", f.Name, err)
		}
	}
	license, _ := assets.ReadFile("assets/LICENSE.VB-CABLE.txt")
	readme, _ := os.ReadFile(filepath.Join(dir, "readme.txt"))
	if !bytes.Equal(license, readme) {
		t.Fatal("upstream license changed")
	}
}

func TestInstallationOutcomesWithoutChangingHost(t *testing.T) {
	for _, tt := range []struct {
		name             string
		before, after    State
		code             uint32
		launchErr        error
		wantErr          bool
		restart, already bool
	}{
		{"ready does not relaunch", Ready, Ready, 0, nil, false, false, true},
		{"installed but inactive does not relaunch", Unavailable, Unavailable, 0, nil, false, false, true},
		{"UAC cancelled", Missing, Missing, 0, ErrCancelled, true, false, false},
		{"wizard cancelled", Missing, Missing, 1602, nil, true, false, false},
		{"wizard failed", Missing, Missing, 5, nil, true, false, false},
		{"closed without installing", Missing, Missing, 0, nil, true, false, false},
		{"reboot required", Missing, Unavailable, 0, nil, false, true, false},
		{"visible endpoints still require upstream reboot", Missing, Ready, 0, nil, false, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			launched := false
			m := &Manager{detect: func() (Status, error) {
				if launched {
					return Status{State: tt.after}, nil
				}
				return Status{State: tt.before}, nil
			}, extract: func(string) (string, error) { return "official.exe", nil }, launch: func(string) (uint32, error) { launched = true; return tt.code, tt.launchErr }, boot: func() (string, error) { return "boot A", nil }}
			r, err := m.Install(t.TempDir())
			if (err != nil) != tt.wantErr || r.RestartRequired != tt.restart || r.AlreadyInstalled != tt.already {
				t.Fatal(r, err)
			}
			if tt.already && launched {
				t.Fatal("installed driver could be removed by a repeated wizard")
			}
			if tt.launchErr != nil && !errors.Is(err, tt.launchErr) {
				t.Fatal("cancellation reason lost", err)
			}
		})
	}
}

func TestRebootRequirementPersistsUntilSystemBootChanges(t *testing.T) {
	dir := t.TempDir()
	current := "boot A"
	boot := func() (string, error) { return current, nil }
	if pending, err := pendingReboot(dir, boot); err != nil || pending {
		t.Fatal(pending, err)
	}
	if err := markReboot(dir, current); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if pending, err := pendingReboot(dir, boot); err != nil || !pending {
			t.Fatal("application restart lost system reboot requirement", pending, err)
		}
	}
	current = "boot B"
	if pending, err := pendingReboot(dir, boot); err != nil || pending {
		t.Fatal("system reboot not recognized", pending, err)
	}
}

func TestWindowsBootTimeIsReadableAndStable(t *testing.T) {
	a, err := bootTime()
	if err != nil {
		t.Fatal(err)
	}
	b, err := bootTime()
	if err != nil || a == "" || a != b {
		t.Fatal("unstable Windows boot identity", a, b, err)
	}
}

func TestBootIdentityIgnoresTimeZoneChanges(t *testing.T) {
	a, err := normalizeBootTime("20260921124931.750778+480")
	if err != nil {
		t.Fatal(err)
	}
	b, err := normalizeBootTime("20260921044931.750778+000")
	if err != nil || a != b {
		t.Fatal(a, b, err)
	}
}

func TestDetectAtPreservesRebootStatusAcrossApplicationRestarts(t *testing.T) {
	dir := t.TempDir()
	stamp, err := bootTime()
	if err != nil {
		t.Fatal(err)
	}
	if err := markReboot(dir, stamp); err != nil {
		t.Fatal(err)
	}
	s, err := DetectAt(dir)
	if err != nil || !s.RestartRequired {
		t.Fatal("same-boot restart acknowledged installation", s, err)
	}
	if err := markReboot(dir, "previous system boot"); err != nil {
		t.Fatal(err)
	}
	s, err = DetectAt(dir)
	if err != nil || s.RestartRequired {
		t.Fatal("system boot change not acknowledged", s, err)
	}
}

func TestDetectionFailurePreventsInstallation(t *testing.T) {
	m := &Manager{detect: func() (Status, error) { return Status{}, errors.New("PnP unavailable") }, extract: func(string) (string, error) { t.Fatal("extracted after failed detection"); return "", nil }}
	if _, err := m.Install("unused"); err == nil {
		t.Fatal("failed detection ignored")
	}
}
