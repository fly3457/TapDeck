//go:build windows

package driver

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"tapdeck/internal/config"
)

//go:embed assets/*
var assets embed.FS

const Filename = "FakerInput_Setup_0.1.1_x64.msi"
const SHA256 = "4c0aefb7340051a91d606776243298b5cd1143ef5508bbae6800c474f9ed0840"

func Verify() error {
	b, e := assets.ReadFile("assets/" + Filename)
	if e != nil {
		return e
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != SHA256 {
		return fmt.Errorf("内置驱动安装包校验失败")
	}
	var m struct {
		SHA256 string `json:"sha256"`
		Bytes  int    `json:"bytes"`
	}
	bm, _ := assets.ReadFile("assets/manifest.json")
	if json.Unmarshal(bm, &m) != nil || m.SHA256 != SHA256 || m.Bytes != len(b) {
		return fmt.Errorf("内置驱动版本清单不匹配")
	}
	return nil
}

// Extract works offline and is idempotent. Only the pinned, original upstream
// MSI is installed; no INF edits or locally signed driver build is involved.
func Extract(dir string) (string, error) {
	if e := Verify(); e != nil {
		return "", e
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	for _, name := range []string{Filename, "LICENSE.FakerInput.txt", "manifest.json"} {
		b, _ := assets.ReadFile("assets/" + name)
		if e := config.AtomicWrite(filepath.Join(dir, name), b); e != nil {
			return "", e
		}
	}
	path, e := filepath.Abs(filepath.Join(dir, Filename))
	return path, e
}

type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            uintptr
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance                          uintptr
	IDList                            uintptr
	Class                             *uint16
	ClassKey                          uintptr
	HotKey                            uint32
	Icon                              uintptr
	Process                           windows.Handle
}
type InstallResult struct {
	Code            uint32
	RestartRequired bool
	Log             string
}

func Install(dir string, repair bool) (InstallResult, error) {
	path, e := Extract(dir)
	if e != nil {
		return InstallResult{}, e
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return InstallResult{}, e
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != SHA256 {
		return InstallResult{}, fmt.Errorf("解出的驱动安装包校验失败")
	}
	if e = VerifySignature(path); e != nil {
		return InstallResult{}, fmt.Errorf("驱动签名校验失败: %w", e)
	}
	logPath := filepath.Join(dir, "install.log")
	params := "/i " + syscall.EscapeArg(path) + " /passive /norestart /l*v " + syscall.EscapeArg(logPath)
	if repair {
		params += " REINSTALL=ALL REINSTALLMODE=vomus"
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(filepath.Join(os.Getenv("WINDIR"), "System32", "msiexec.exe"))
	args, _ := windows.UTF16PtrFromString(params)
	info := shellExecuteInfo{Mask: 0x40 | 0x100, Verb: verb, File: file, Parameters: args, Show: 0}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, e := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if e == windows.ERROR_CANCELLED {
			return InstallResult{}, fmt.Errorf("已取消管理员授权，未安装驱动")
		}
		return InstallResult{}, fmt.Errorf("无法启动驱动安装: %w", e)
	}
	defer windows.CloseHandle(info.Process)
	if _, e = windows.WaitForSingleObject(info.Process, windows.INFINITE); e != nil {
		return InstallResult{}, e
	}
	var code uint32
	if e = windows.GetExitCodeProcess(info.Process, &code); e != nil {
		return InstallResult{}, e
	}
	r := InstallResult{Code: code, RestartRequired: code == 3010 || code == 1641, Log: logPath}
	if code != 0 && !r.RestartRequired {
		if code == 1602 {
			return r, fmt.Errorf("安装已取消")
		}
		return r, fmt.Errorf("驱动安装失败，MSI 返回 %d，日志：%s", code, logPath)
	}
	return r, nil
}
func VerifySignature(path string) error {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return e
	}
	f := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: p}
	d := windows.WinTrustData{Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: windows.WTD_UI_NONE, RevocationChecks: windows.WTD_REVOKE_NONE, UnionChoice: windows.WTD_CHOICE_FILE, StateAction: windows.WTD_STATEACTION_VERIFY, ProvFlags: windows.WTD_CACHE_ONLY_URL_RETRIEVAL, FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&f)}
	err := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &d)
	d.StateAction = windows.WTD_STATEACTION_CLOSE
	_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &d)
	return err
}
