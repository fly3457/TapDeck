//go:build windows

// keywatch 是排查「手机按键到底有没有到 PC」的诊断工具：轮询 GetAsyncKeyState，
// 打印这段时间内所有真实下按的虚拟键（与输入法、焦点窗口无关）。
//
//	go run ./cmd/keywatch -seconds 8
package main

import (
	"flag"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	getAsyncKeyState  = user32.NewProc("GetAsyncKeyState")
	getKeyNameText    = user32.NewProc("GetKeyNameTextW")
	mapVirtualKeyProc = user32.NewProc("MapVirtualKeyW")
)

func keyName(vk int) string {
	scan, _, _ := mapVirtualKeyProc.Call(uintptr(vk), 0)
	buf := make([]uint16, 64)
	n, _, _ := getKeyNameText.Call(uintptr(scan<<16), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return fmt.Sprintf("VK_%02X", vk)
	}
	return windows.UTF16ToString(buf)
}

func down(vk int) bool {
	v, _, _ := getAsyncKeyState.Call(uintptr(vk))
	return uint16(v)&0x8000 != 0
}

func main() {
	seconds := flag.Int("seconds", 8, "轮询时长（秒）")
	flag.Parse()

	type event struct {
		at   time.Duration
		vk   int
		down bool
	}
	var events []event
	start := time.Now()
	deadline := start.Add(time.Duration(*seconds) * time.Second)
	state := map[int]bool{}
	for time.Now().Before(deadline) {
		for vk := 0x08; vk <= 0xFE; vk++ {
			now := down(vk)
			if now == state[vk] {
				continue
			}
			state[vk] = now
			events = append(events, event{time.Since(start), vk, now})
		}
		time.Sleep(4 * time.Millisecond)
	}

	var b strings.Builder
	for _, e := range events {
		kind := "UP  "
		if e.down {
			kind = "DOWN"
		}
		fmt.Fprintf(&b, "%7dms %s VK 0x%02X %s\n", e.at.Milliseconds(), kind, e.vk, keyName(e.vk))
	}
	fmt.Print(b.String())
	fmt.Printf("共 %d 个事件\n", len(events))
}
