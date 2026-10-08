//go:build windows

// Package vbcable distributes the unmodified upstream Pack45 and launches its
// interactive installer. It never changes the default recording device.
package vbcable

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"tapdeck/internal/audio"
	"tapdeck/internal/config"
	"tapdeck/internal/driver"
)

const Filename = "VBCABLE_Driver_Pack45.zip"
const SHA256 = "b950e39f01af1d04ea623c8f6d8eb9b6ea5c477c637295fabf20631c85116bfb"
const Website = "https://vb-audio.com/Cable/"

//go:embed assets/*
var assets embed.FS

type State string

const (
	Missing     State = "missing"
	Unavailable State = "installed_unavailable"
	Ready       State = "ready"
)

type Status struct {
	State           State  `json:"state"`
	DriverInstalled bool   `json:"driver_installed"`
	RenderID        string `json:"render_id,omitempty"`
	CaptureID       string `json:"capture_id,omitempty"`
	RestartRequired bool   `json:"restart_required,omitempty"`
}

func (s Status) Text() string {
	if s.RestartRequired {
		return "VB-CABLE 已安装，仍需重启 Windows 后重新检测；重开 TapDeck 不会完成此步骤"
	}
	switch s.State {
	case Missing:
		return "未安装 VB-CABLE，可从此处离线安装"
	case Ready:
		return "VB-CABLE 可用：CABLE Input / CABLE Output 均已就绪"
	default:
		return "VB-CABLE 已安装但端点不可用，请重启 Windows 或在声音设置中启用端点后重新检测"
	}
}

// classify deliberately considers the PnP driver even when no active WASAPI
// endpoints exist (disabled devices or installation awaiting a reboot).
func classify(installed bool, render, capture []audio.Device) Status {
	s := Status{DriverInstalled: installed, State: Missing}
	for _, d := range render {
		if strings.Contains(strings.ToUpper(d.Name), "CABLE INPUT") {
			s.RenderID = d.ID
			break
		}
	}
	for _, d := range capture {
		if strings.Contains(strings.ToUpper(d.Name), "CABLE OUTPUT") {
			s.CaptureID = d.ID
			break
		}
	}
	if s.DriverInstalled || s.RenderID != "" || s.CaptureID != "" {
		s.State = Unavailable
	}
	if s.RenderID != "" && s.CaptureID != "" {
		s.State = Ready
	}
	return s
}

func Detect() (Status, error) {
	installed, err := driverInstalled()
	if err != nil {
		return Status{}, err
	}
	render, err := audio.Devices()
	if err != nil {
		return Status{}, err
	}
	capture, err := audio.CaptureDevices()
	if err != nil {
		return Status{}, err
	}
	return classify(installed, render, capture), nil
}

func driverInstalled() (bool, error) {
	// Include non-present devices to recognize the installed driver while its
	// endpoints are disabled or have not been published after installation.
	set, err := windows.SetupDiGetClassDevsEx(nil, "", 0, windows.DIGCF_ALLCLASSES, 0, "")
	if err != nil {
		return false, err
	}
	defer set.Close()
	for i := 0; ; i++ {
		info, err := set.EnumDeviceInfo(i)
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		hardware, _ := set.DeviceRegistryProperty(info, windows.SPDRP_HARDWAREID)
		service, _ := set.DeviceRegistryProperty(info, windows.SPDRP_SERVICE)
		ids, _ := hardware.([]string)
		for _, id := range ids {
			if strings.EqualFold(id, "VBAudioVACWDM") && strings.EqualFold(fmt.Sprint(service), "VBAudioVACMME") {
				return true, nil
			}
		}
	}
}

func Verify() error {
	b, err := assets.ReadFile("assets/" + Filename)
	if err != nil {
		return err
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != SHA256 {
		return fmt.Errorf("内置 VB-CABLE 原包 SHA-256 校验失败")
	}
	return nil
}

func License(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "LICENSE.VB-CABLE.txt")
	b, err := assets.ReadFile("assets/LICENSE.VB-CABLE.txt")
	if err != nil {
		return "", err
	}
	return path, config.AtomicWrite(path, b)
}

// Extract preserves every file of the original ZIP, including the EULA.
func Extract(dir string) (string, error) {
	if err := Verify(); err != nil {
		return "", err
	}
	b, _ := assets.ReadFile("assets/" + Filename)
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for _, f := range z.File {
		name := filepath.FromSlash(f.Name)
		path := filepath.Join(dir, name)
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(name) {
			return "", fmt.Errorf("安装包包含越界路径")
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0700); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		r, err := f.Open()
		if err != nil {
			return "", err
		}
		contents, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			return "", err
		}
		if err := config.AtomicWrite(path, contents); err != nil {
			return "", err
		}
	}
	setup := filepath.Join(dir, "VBCABLE_Setup_x64.exe")
	for _, path := range []string{setup, filepath.Join(dir, "vbaudio_cable64_win10.cat")} {
		if err := driver.VerifySignature(path); err != nil {
			return "", fmt.Errorf("VB-CABLE 签名校验失败（%s）: %w", filepath.Base(path), err)
		}
	}
	return setup, nil
}

var ErrCancelled = errors.New("已取消 VB-CABLE 安装或管理员授权")

type InstallResult struct {
	Status           Status
	Code             uint32
	AlreadyInstalled bool
	RestartRequired  bool
}

// Manager's dependencies can be replaced in tests without touching drivers.
type Manager struct {
	mu      sync.Mutex
	detect  func() (Status, error)
	extract func(string) (string, error)
	launch  func(string) (uint32, error)
	boot    func() (string, error)
}

func NewManager() *Manager {
	return &Manager{detect: Detect, extract: Extract, launch: launchInstaller, boot: bootTime}
}

func (m *Manager) Install(dir string) (InstallResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.detect()
	if err != nil {
		return InstallResult{}, fmt.Errorf("检测失败，未启动安装: %w", err)
	}
	// The official wizard offers removal when a driver exists. Never launch
	// it automatically as a repair action on an already installed system.
	if s.State != Missing {
		return InstallResult{Status: s, AlreadyInstalled: true}, nil
	}
	setup, err := m.extract(dir)
	if err != nil {
		return InstallResult{}, err
	}
	s, err = m.detect()
	if err != nil {
		return InstallResult{}, err
	}
	if s.State != Missing {
		return InstallResult{Status: s, AlreadyInstalled: true}, nil
	}
	stamp, err := m.boot()
	if err != nil {
		return InstallResult{}, fmt.Errorf("读取系统启动时间失败，未启动安装: %w", err)
	}
	code, err := m.launch(setup)
	if err != nil {
		return InstallResult{}, err
	}
	r := InstallResult{Code: code}
	if code == 1602 {
		return r, ErrCancelled
	}
	if code != 0 && code != 3010 && code != 1641 {
		return r, fmt.Errorf("VB-CABLE 安装向导返回 %d", code)
	}
	r.Status, err = m.detect()
	if err != nil {
		return r, fmt.Errorf("安装后重新检测失败: %w", err)
	}
	if r.Status.State == Missing {
		return r, fmt.Errorf("安装向导已关闭，但未检测到 VB-CABLE；安装可能已取消或失败，请查看官方向导结果")
	}
	// The upstream documentation requires a reboot even if endpoints appear
	// immediately. Only a detection after that reboot completes acceptance.
	r.RestartRequired = true
	r.Status.RestartRequired = true
	if err := markReboot(dir, stamp); err != nil {
		return r, fmt.Errorf("VB-CABLE 已安装，仍需重启 Windows；保存重启记录失败: %w", err)
	}
	return r, nil
}

type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            uintptr
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance, IDList                  uintptr
	Class                             *uint16
	ClassKey                          uintptr
	HotKey                            uint32
	Icon                              uintptr
	Process                           windows.Handle
}

func launchInstaller(path string) (uint32, error) {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(path)
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(path))
	info := shellExecuteInfo{Mask: 0x40 | 0x100, Verb: verb, File: file, Directory: dir, Show: 1}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, err := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return 0, ErrCancelled
		}
		return 0, fmt.Errorf("无法启动 VB-CABLE 官方安装向导: %w", err)
	}
	defer windows.CloseHandle(info.Process)
	if _, err := windows.WaitForSingleObject(info.Process, windows.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	err = windows.GetExitCodeProcess(info.Process, &code)
	return code, err
}
