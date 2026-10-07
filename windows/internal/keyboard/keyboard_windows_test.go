//go:build windows

package keyboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

type fakeSoft struct {
	down, up []uint16
	owned    map[uint16]int
}

func (s *fakeSoft) HoldKeys(ks []uint16, down bool) error {
	for _, k := range ks {
		if down {
			s.down = append(s.down, k)
			s.owned[k]++
		} else {
			s.up = append(s.up, k)
			s.owned[k]--
		}
	}
	return nil
}
func (s *fakeSoft) ReleaseAll() { s.owned = map[uint16]int{} }
func fixture() (*Engine, *fakeSoft, *[]map[uint16]int) {
	s := &fakeSoft{owned: map[uint16]int{}}
	reports := []map[uint16]int{}
	e := &Engine{mode: "auto", soft: s, hidKeys: map[uint16]int{}, owners: map[uint16]keyOwner{}, holds: map[string][]string{}, voices: map[string]voice{}}
	e.writeHID = func(ks map[uint16]int) error { reports = append(reports, copyKeys(ks)); return nil }
	return e, s, &reports
}

// 键盘上没有的字符（…）只能走 SendInput：虚拟键盘不支持，HID 模式下应当报错。
func TestUnicodeOnlyKeyFallsBackToSendInput(t *testing.T) {
	e, s, reports := fixture()
	if err := e.Hold("Ellipsis", true); err != nil {
		t.Fatal(err)
	}
	if len(s.down) != 1 || s.down[0] != 0xE000 {
		t.Fatal("unicode key not routed to SendInput", s)
	}
	if len(*reports) != 0 {
		t.Fatal("unicode key must not be written to the HID device", *reports)
	}
	if err := e.Hold("Ellipsis", false); err != nil {
		t.Fatal(err)
	}
	if len(s.up) != 1 || s.up[0] != 0xE000 {
		t.Fatal("unicode key not released", s)
	}
	hidEngine, _, _ := fixture()
	hidEngine.mode = "hid"
	if err := Validate("LeftShift+Ellipsis", "hid"); err == nil {
		t.Fatal("hid mode should reject the unicode-only key")
	}
	if err := Validate("LeftShift+Ellipsis", "auto"); err != nil {
		t.Fatal("auto mode should accept the unicode-only key", err)
	}
}

func TestSharedModifierAcrossHIDAndUnsupportedFallback(t *testing.T) {
	e, s, reports := fixture()
	if err := e.Hold("LeftCtrl+M", true); err != nil {
		t.Fatal(err)
	}
	if err := pulse(context.Background(), e, "Ctrl+F24"); err != nil {
		t.Fatal(err)
	}
	if len(s.down) != 1 || s.down[0] != 0x87 || len(s.up) != 1 || s.up[0] != 0x87 {
		t.Fatal("fallback released HID Ctrl", s)
	}
	if len(*reports) != 1 || e.hidKeys[0xA2] != 1 {
		t.Fatal("fallback altered hardware-held Ctrl", *reports)
	}
	if err := pulse(context.Background(), e, "Ctrl+C"); err != nil {
		t.Fatal(err)
	}
	if e.hidKeys[0xA2] != 1 || e.hidKeys['M'] != 1 {
		t.Fatal("shortcut released voice chord")
	}
	if err := e.Hold("Ctrl+M", false); err != nil {
		t.Fatal(err)
	}
	if len(e.hidKeys) != 0 || len(e.owners) != 0 {
		t.Fatal("aliases failed to release")
	}
}

func TestVolumeKeysUseSoftwareControllerInEveryKeyboardMode(t *testing.T) {
	for _, mode := range []string{"auto", "hid", "sendinput"} {
		t.Run(mode, func(t *testing.T) {
			e, soft, reports := fixture()
			e.mode = mode
			for _, chord := range []string{"VolumeUp", "VolumeDown", "VolumeMute"} {
				if err := Validate(chord, mode); err != nil {
					t.Fatal("volume action rejected by HID validation", err)
				}
				if err := pulse(context.Background(), e, chord); err != nil {
					t.Fatal(err)
				}
			}
			if len(*reports) != 0 || len(soft.down) != 3 || len(soft.up) != 3 || len(e.owners) != 0 {
				t.Fatal("volume action entered HID or leaked held state", *reports, soft)
			}
		})
	}
}

func TestVolumeShortcutLeavesHIDHeldModifierUntouched(t *testing.T) {
	e, soft, reports := fixture()
	e.mode = "hid"
	if err := e.Hold("LeftCtrl+C", true); err != nil {
		t.Fatal(err)
	}
	if err := pulse(context.Background(), e, "Ctrl+VolumeUp"); err != nil {
		t.Fatal(err)
	}
	if len(*reports) != 1 || e.hidKeys[0xA2] != 1 || e.hidKeys['C'] != 1 || len(soft.down) != 1 || soft.down[0] != 0xAF {
		t.Fatal("volume shortcut changed the HID-held keys", *reports, soft)
	}
	if err := e.Hold("LeftCtrl+C", false); err != nil {
		t.Fatal(err)
	}
}
func TestReverseBackendReferenceOwnership(t *testing.T) {
	e, s, _ := fixture()
	if err := e.Hold("Ctrl+F24", true); err != nil {
		t.Fatal(err)
	}
	if err := pulse(context.Background(), e, "Ctrl+C"); err != nil {
		t.Fatal(err)
	}
	if len(s.up) != 0 || s.owned[0xA2] != 1 || e.hidKeys[0xA2] != 0 {
		t.Fatal("HID shortcut released software-held Ctrl")
	}
	if err := e.Hold("LeftCtrl+F24", false); err != nil {
		t.Fatal(err)
	}
	if s.owned[0xA2] != 0 || s.owned[0x87] != 0 {
		t.Fatal("software held keys leaked")
	}
	if err := Validate("F13", "hid"); err == nil {
		t.Fatal("forced HID accepted unsupported descriptor key")
	}
}
func TestVoiceSnapshotsAndNoLateToggleEnd(t *testing.T) {
	e, _, reports := fixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.startVoice(ctx, request{Token: "old", Mode: "toggle", Chord: "Ctrl+L", Stop: "Ctrl+L"}); err == nil {
		t.Fatal("canceled start accepted")
	}
	e.stopVoice(context.Background(), "old")
	if len(*reports) != 0 {
		t.Fatal("canceled preparation triggered a stop/start hotkey")
	}
	if err := e.startVoice(context.Background(), request{Token: "new", Mode: "toggle", Chord: "Ctrl+L", Stop: "RightAlt"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Configure("sendinput"); err == nil {
		t.Fatal("backend switched during voice")
	}
	if err := e.stopVoice(context.Background(), "new"); err != nil {
		t.Fatal(err)
	}
	n := len(*reports)
	e.stopVoice(context.Background(), "new")
	e.clearVoices()
	if len(*reports) != n+1 {
		t.Fatal("duplicate stop hotkey", *reports)
	} // clear sends only an empty report
}
func TestHIDWriteFailureIsNeverReplayed(t *testing.T) {
	e, s, _ := fixture()
	e.writeHID = func(map[uint16]int) error { return fmt.Errorf("device removed") }
	if err := e.Hold("Ctrl+M", true); err == nil {
		t.Fatal("write error lost")
	}
	if len(s.down) != 0 || e.Status().Ready {
		t.Fatal("failed hardware chord replayed using SendInput")
	}
}
func TestInheritedPipeEOFReleasesVoiceAndDropsQueue(t *testing.T) {
	e, _, reports := fixture()
	var mu sync.Mutex
	original := e.writeHID
	e.writeHID = func(k map[uint16]int) error { mu.Lock(); defer mu.Unlock(); return original(k) }
	rin, win := io.Pipe()
	rout, wout := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- runWorker(rin, wout, e); wout.Close() }()
	responses := make(chan response, 64)
	go func() {
		dec := json.NewDecoder(rout)
		for {
			var r response
			if dec.Decode(&r) != nil {
				return
			}
			responses <- r
		}
	}()
	<-responses
	enc := json.NewEncoder(win)
	if err := enc.Encode(request{ID: 1, Epoch: 1, Action: "voice_start", Token: "record", Mode: "hold", Chord: "RightCtrl+M"}); err != nil {
		t.Fatal(err)
	}
	for {
		r := <-responses
		if r.ID == 1 {
			if r.Error != "" {
				t.Fatal(r.Error)
			}
			break
		}
	}
	win.Close() // simulates parent process death without a shutdown message
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("orphaned keyboard worker")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*reports) < 2 || len((*reports)[len(*reports)-1]) != 0 {
		t.Fatal("held Ctrl/M survived pipe EOF", *reports)
	}
}
