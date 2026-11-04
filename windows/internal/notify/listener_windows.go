//go:build windows

package notify

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"tapdeck/internal/winrt"
)

var (
	iidListenerStatics   = winrt.GUID("ff6123cf-4386-4aa3-b73d-b804e5b63b23")
	iidAdaptiveText      = winrt.GUID("46d4a3be-609a-4326-a40b-bfde872034a3")
	toastGenericTemplate = "ToastGeneric"
)

// Access 是 Windows 通知访问权限状态。
type Access int

const (
	AccessUnknown Access = iota
	AccessAllowed
	AccessDenied
	AccessUnspecified
	AccessUnavailable
)

// Text 返回设置界面显示的状态说明。
func (a Access) Text() string {
	switch a {
	case AccessAllowed:
		return "已允许读取 Windows 通知"
	case AccessDenied:
		return "Windows 已拒绝读取通知：请在“设置 → 隐私和安全性 → 通知”中允许应用访问通知"
	case AccessUnspecified:
		return "尚未授权读取 Windows 通知：请在“设置 → 隐私和安全性 → 通知”中允许"
	case AccessUnavailable:
		return "此 Windows 版本不支持读取通知"
	default:
		return "正在检测通知访问权限…"
	}
}

// listener 包装 UserNotificationListener.Current。
type listener struct{ obj *winrt.Object }

func openListener() (*listener, error) {
	statics, err := winrt.Factory("Windows.UI.Notifications.Management.UserNotificationListener", &iidListenerStatics)
	if err != nil {
		return nil, err
	}
	defer statics.Release()
	current, err := statics.Get(6) // get_Current
	if err != nil {
		return nil, err
	}
	return &listener{current}, nil
}

func (l *listener) release() { l.obj.Release() }

func (l *listener) access() Access {
	var v int32
	if winrt.Check(l.obj.Call(7, uintptr(unsafe.Pointer(&v)))) != nil { // GetAccessStatus
		return AccessUnavailable
	}
	switch v {
	case 1:
		return AccessAllowed
	case 2:
		return AccessDenied
	default:
		return AccessUnspecified
	}
}

// requestAccess 在状态未指定时请求授权；桌面应用通常直接返回系统隐私设置的结果。
func (l *listener) requestAccess() {
	op, err := l.obj.Get(6) // RequestAccessAsync
	if err != nil {
		return
	}
	defer op.Release()
	_ = winrt.Await(op, 30*time.Second)
}

// snapshot 读取通知中心当前的全部 Toast。
func (l *listener) snapshot() ([]*winrt.Object, error) {
	op, err := l.obj.Get(10, 1) // GetNotificationsAsync(NotificationKinds.Toast)
	if err != nil {
		return nil, err
	}
	defer op.Release()
	view, err := winrt.Result(op, 10*time.Second)
	if err != nil {
		return nil, err
	}
	defer view.Release()
	return winrt.Items(view)
}

func userNotificationID(un *winrt.Object) (uint32, error) {
	var id uint32
	return id, winrt.Check(un.Call(8, uintptr(unsafe.Pointer(&id)))) // get_Id
}

// read 提取应用名、时间、标题与正文。
func read(un *winrt.Object) Notification {
	n := Notification{Time: time.Now().UnixMilli()}
	var created int64
	if winrt.Check(un.Call(9, uintptr(unsafe.Pointer(&created)))) == nil && created > 0 { // get_CreationTime
		// DateTime 为 1601-01-01 起的 100ns 计数。
		n.Time = (created - 116444736000000000) / 10000
	}
	if info, err := un.Get(7); err == nil { // get_AppInfo
		if display, err := info.Get(8); err == nil { // get_DisplayInfo
			n.App, _ = display.GetString(6) // get_DisplayName
			display.Release()
		}
		if strings.TrimSpace(n.App) == "" {
			// 注册表登记的未打包应用（含 TapDeck 自己）没有显示信息，退回 AppUserModelId。
			aumid, _ := info.GetString(7) // get_AppUserModelId
			n.App = appNameFromID(aumid)
		}
		info.Release()
	}
	texts := textElements(un)
	if len(texts) > 0 {
		n.Title = texts[0]
		n.Body = strings.Join(texts[1:], "\n")
	}
	return n.Normalize()
}

func textElements(un *winrt.Object) []string {
	notification, err := un.Get(6) // get_Notification
	if err != nil {
		return nil
	}
	defer notification.Release()
	visual, err := notification.Get(8) // get_Visual
	if err != nil {
		return nil
	}
	defer visual.Release()
	template, err := winrt.NewString(toastGenericTemplate)
	if err != nil {
		return nil
	}
	defer template.Delete()
	binding, err := visual.Get(9, uintptr(template)) // GetBinding
	if err != nil {
		return nil
	}
	defer binding.Release()
	list, err := binding.Get(11) // GetTextElements
	if err != nil {
		return nil
	}
	defer list.Release()
	items, err := winrt.Items(list)
	if err != nil {
		return nil
	}
	var out []string
	for _, item := range items {
		if text, err := item.Query(&iidAdaptiveText); err == nil {
			if s, err := text.GetString(6); err == nil && strings.TrimSpace(s) != "" { // get_Text
				out = append(out, s)
			}
			text.Release()
		}
		item.Release()
	}
	return out
}

// Watch 每隔 interval 读取一次通知中心，把新出现的通知交给 emit；status 在权限
// 状态变化时回调。启动时已存在的通知只作为基线。ctx 结束后返回。
func Watch(ctx context.Context, interval time.Duration, emit func(Notification), status func(Access)) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := winrt.Init(); err != nil {
		status(AccessUnavailable)
		return fmt.Errorf("WinRT 初始化失败：%w", err)
	}
	l, err := openListener()
	if err != nil {
		status(AccessUnavailable)
		return fmt.Errorf("无法打开通知监听器：%w", err)
	}
	defer l.release()
	var differ Differ
	last := AccessUnknown
	requested := false
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		a := l.access()
		if a == AccessUnspecified && !requested {
			requested = true
			l.requestAccess()
			a = l.access()
		}
		if a != last {
			last = a
			status(a)
			// 权限恢复后重新建立基线，不补发关闭期间积累的通知。
			differ = Differ{}
		}
		if a == AccessAllowed {
			poll(l, &differ, emit)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func poll(l *listener, differ *Differ, emit func(Notification)) {
	items, err := l.snapshot()
	if err != nil {
		return
	}
	defer func() {
		for _, v := range items {
			v.Release()
		}
	}()
	ids := make([]uint32, 0, len(items))
	byID := make(map[uint32]*winrt.Object, len(items))
	for _, un := range items {
		if id, err := userNotificationID(un); err == nil {
			ids = append(ids, id)
			byID[id] = un
		}
	}
	for _, id := range differ.Next(ids) {
		if n := read(byID[id]); !n.Empty() {
			emit(n)
		}
	}
}
