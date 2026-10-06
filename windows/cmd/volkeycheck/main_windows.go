//go:build windows

// volkeycheck verifies that the volume chords routed through the keyboard engine
// really change the system volume, and reports the before/after state.
package main

import (
	"fmt"
	"time"

	"tapdeck/internal/audio"
	"tapdeck/internal/input"
)

func main() {
	vol, err := audio.NewVolumeControl()
	if err != nil {
		fmt.Println("volume control:", err)
		return
	}
	defer vol.Close()
	before, err := vol.Level()
	if err != nil {
		fmt.Println("level:", err)
		return
	}
	step, count, _ := vol.StepInfo()
	fmt.Printf("before: level=%.4f step=%d/%d\n", before, step, count)

	c := input.New()
	c.SetVolume(vol)
	if err := c.Chord("VolumeUp"); err != nil {
		fmt.Println("VolumeUp:", err)
		return
	}
	time.Sleep(400 * time.Millisecond)
	raised, _ := vol.Level()
	if err := c.Chord("VolumeDown"); err != nil {
		fmt.Println("VolumeDown:", err)
		return
	}
	time.Sleep(400 * time.Millisecond)
	lowered, _ := vol.Level()
	fmt.Printf("after:  raised=%.4f lowered=%.4f\n", raised, lowered)
	if raised <= before || lowered >= raised {
		fmt.Println("FAIL: volume did not move")
		return
	}
	fmt.Println("OK: VolumeUp / VolumeDown change the system volume")
}
