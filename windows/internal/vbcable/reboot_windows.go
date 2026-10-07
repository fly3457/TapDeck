//go:build windows

package vbcable

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"tapdeck/internal/config"
)

type rebootMarker struct {
	BootTime string `json:"installed_on_boot"`
}

var bootMu sync.Mutex
var cachedBoot string

func markReboot(dir, boot string) error {
	b, err := json.Marshal(rebootMarker{BootTime: boot})
	if err != nil {
		return err
	}
	return config.AtomicWrite(filepath.Join(dir, "tapdeck-reboot.json"), b)
}

func pendingReboot(dir string, boot func() (string, error)) (bool, error) {
	b, err := os.ReadFile(filepath.Join(dir, "tapdeck-reboot.json"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	var marker rebootMarker
	if err := json.Unmarshal(b, &marker); err != nil {
		return true, err
	}
	if marker.BootTime == "" {
		return true, fmt.Errorf("VB-CABLE 重启记录无效")
	}
	current, err := boot()
	if err != nil {
		return true, err
	}
	return marker.BootTime == current, nil
}

// DetectAt keeps an installation awaiting a system reboot visible across
// application restarts. Active endpoints alone do not acknowledge the reboot.
func DetectAt(dir string) (Status, error) {
	s, err := Detect()
	if err != nil {
		return s, err
	}
	s.RestartRequired, err = pendingReboot(dir, bootTime)
	return s, err
}

// Win32_OperatingSystem.LastBootUpTime is stable for the current Windows boot;
// querying on a separate MTA thread also works from Walk's STA settings UI.
func bootTime() (string, error) {
	bootMu.Lock()
	defer bootMu.Unlock()
	// A process cannot survive a Windows restart. Cache only successful
	// reads for this process, avoiding repeated COM/WMI startup and teardown.
	if cachedBoot != "" {
		return cachedBoot, nil
	}
	type result struct {
		value string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
			done <- result{err: err}
			return
		}
		defer ole.CoUninitialize()
		value, err := queryBootTime()
		done <- result{value, err}
	}()
	r := <-done
	if r.err == nil {
		cachedBoot = r.value
	}
	return r.value, r.err
}

func queryBootTime() (string, error) {
	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return "", err
	}
	defer unknown.Release()
	locator, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return "", err
	}
	defer locator.Release()
	connection, err := oleutil.CallMethod(locator, "ConnectServer", ".", "root\\cimv2")
	if err != nil {
		return "", err
	}
	defer connection.Clear()
	rows, err := oleutil.CallMethod(connection.ToIDispatch(), "ExecQuery", "SELECT LastBootUpTime FROM Win32_OperatingSystem")
	if err != nil {
		return "", err
	}
	defer rows.Clear()
	row, err := oleutil.CallMethod(rows.ToIDispatch(), "ItemIndex", 0)
	if err != nil {
		return "", err
	}
	defer row.Clear()
	value, err := oleutil.GetProperty(row.ToIDispatch(), "LastBootUpTime")
	if err != nil {
		return "", err
	}
	defer value.Clear()
	stamp := value.ToString()
	if stamp == "" {
		return "", fmt.Errorf("Windows 未返回系统启动时间")
	}
	return normalizeBootTime(stamp)
}

func normalizeBootTime(stamp string) (string, error) {
	if len(stamp) != 25 || (stamp[21] != '+' && stamp[21] != '-') {
		return "", fmt.Errorf("Windows 系统启动时间格式无效")
	}
	local, err := time.Parse("20060102150405.000000", stamp[:21])
	if err != nil {
		return "", err
	}
	minutes, err := strconv.Atoi(stamp[22:])
	if err != nil {
		return "", err
	}
	if stamp[21] == '-' {
		minutes = -minutes
	}
	return local.Add(-time.Duration(minutes) * time.Minute).UTC().Format(time.RFC3339Nano), nil
}
