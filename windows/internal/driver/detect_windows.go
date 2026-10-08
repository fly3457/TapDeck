//go:build windows

package driver

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows"
)

type State string

const (
	Missing     State = "missing"
	Unavailable State = "installed_unavailable"
	Ready       State = "ready"
)

// Detect combines the worker's real HID availability with installed PnP
// identity. A disabled or not-yet-ready driver must not be called uninstalled.
func Detect(available bool) (State, error) {
	if available {
		return Ready, nil
	}
	set, err := windows.SetupDiGetClassDevsEx(nil, "", 0, windows.DIGCF_ALLCLASSES, 0, "")
	if err != nil {
		return "", err
	}
	defer set.Close()
	for i := 0; ; i++ {
		info, err := set.EnumDeviceInfo(i)
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			return Missing, nil
		}
		if err != nil {
			return "", err
		}
		hardware, err := set.DeviceRegistryProperty(info, windows.SPDRP_HARDWAREID)
		if err != nil {
			if errors.Is(err, windows.ERROR_INVALID_DATA) || errors.Is(err, windows.ERROR_NOT_FOUND) {
				continue // Some unrelated devices have no hardware IDs.
			}
			return "", err
		}
		ids, _ := hardware.([]string)
		if isFakerInput(ids) {
			return Unavailable, nil
		}
	}
}

func isFakerInput(ids []string) bool {
	for _, id := range ids {
		if strings.EqualFold(id, `root\FakerInput`) {
			return true
		}
	}
	return false
}

func (s State) Text() string {
	switch s {
	case Ready:
		return "虚拟键盘已安装并可用"
	case Missing:
		return "未安装虚拟键盘，可从此处离线安装"
	default:
		return "虚拟键盘已安装但不可用，可重新检测或修复驱动"
	}
}
