//go:build windows

// volkeycheck verifies the software controller's Core Audio volume actions.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"tapdeck/internal/audio"
	"tapdeck/internal/input"
)

func main() {
	chord := flag.String("chord", "", "perform one VolumeUp, VolumeDown or VolumeMute action")
	flag.Parse()
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
	if *chord != "" {
		if *chord != "VolumeUp" && *chord != "VolumeDown" && *chord != "VolumeMute" {
			fmt.Fprintln(os.Stderr, "unsupported volume action")
			os.Exit(1)
		}
		if err := c.Chord(*chord); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		after, err := vol.Level()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("%s: %.4f -> %.4f\n", *chord, before, after)
		return
	}
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
