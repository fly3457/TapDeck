//go:build windows

package audio

import (
	"syscall"
	"testing"

	ole "github.com/go-ole/go-ole"
)

func TestVolumeTracksDefaultEndpointAndRecoversAfterRemoval(t *testing.T) {
	volumes := []*com{{VT: new([20]uintptr)}, {VT: new([20]uintptr)}}
	devices := []*com{{VT: new([20]uintptr)}, {VT: new([20]uintptr)}}
	steps := [2]int{}
	deviceReleases, volumeReleases := 0, 0
	for i := range volumes {
		index := i
		volumes[i].VT[slotStepUp] = syscall.NewCallback(func(_ *com, _ uintptr) uintptr { steps[index]++; return 0 })
		volumes[i].VT[slotStepDown] = syscall.NewCallback(func(_ *com, _ uintptr) uintptr { steps[index]--; return 0 })
		volumes[i].VT[2] = syscall.NewCallback(func(_ *com) uintptr { volumeReleases++; return 0 })
		devices[i].VT[3] = syscall.NewCallback(func(_ *com, _ *ole.GUID, _ uint32, _ uintptr, result **com) uintptr {
			*result = volumes[index]
			return 0
		})
		devices[i].VT[2] = syscall.NewCallback(func(_ *com) uintptr { deviceReleases++; return 0 })
	}
	current, defaultCalls, enumerationCalls := 0, 0, 0
	e := &com{VT: new([20]uintptr)}
	e.VT[3] = syscall.NewCallback(func(_ *com, _ uint32, _ uint32, _ **com) uintptr {
		enumerationCalls++
		return 0x80004005
	})
	e.VT[4] = syscall.NewCallback(func(_ *com, flow uint32, role uint32, result **com) uintptr {
		defaultCalls++
		if flow != 0 || role != 0 {
			return 0x80070057
		}
		if current < 0 {
			return 0x80070490
		}
		*result = devices[current]
		return 0
	})
	v := &VolumeControl{endpoint: func(action func(*com) error) error {
		vol, err := defaultVolume(e)
		if err != nil {
			return err
		}
		defer release(vol)
		return action(vol)
	}}
	if err := v.Step(true); err != nil {
		t.Fatal(err)
	}
	current = 1 // Windows switched from speakers to headphones.
	if err := v.Step(false); err != nil {
		t.Fatal(err)
	}
	current = -1
	if err := v.Step(true); err == nil {
		t.Fatal("missing default endpoint was ignored")
	}
	current = 1
	if err := v.Step(true); err != nil {
		t.Fatal("reconnected endpoint stayed unavailable", err)
	}
	if steps != [2]int{1, 0} || defaultCalls != 4 || enumerationCalls != 0 || deviceReleases != 3 || volumeReleases != 3 {
		t.Fatal("wrong endpoint, cached interface or unbalanced release", steps, defaultCalls, enumerationCalls, deviceReleases, volumeReleases)
	}
	v.Close()
	if err := v.Step(true); err == nil || defaultCalls != 4 {
		t.Fatal("closed controller accepted another operation")
	}
}

func TestVolumeReadDefaultPlaybackWithoutChangingIt(t *testing.T) {
	v, err := NewVolumeControl()
	if err != nil {
		t.Skipf("no default playback endpoint: %v", err)
	}
	defer v.Close()
	level, err := v.Level()
	if err != nil {
		t.Fatal(err)
	}
	if level < 0 || level > 1 {
		t.Fatal("invalid default volume", level)
	}
	if _, _, err := v.StepInfo(); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Mute(); err != nil {
		t.Fatal(err)
	}
}
