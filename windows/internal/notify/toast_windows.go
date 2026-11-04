//go:build windows

package notify

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"
	"tapdeck/internal/winrt"
)

var (
	iidToastManagerStatics = winrt.GUID("50ac103f-d235-4598-bbef-98fe4d1a3ad4")
	iidToastFactory        = winrt.GUID("04124b20-82c6-4229-b109-fd9ed4662b53")
	iidXmlDocument         = winrt.GUID("f7f3a506-1e87-42d6-bcfb-b8c809fa5494")
	iidXmlDocumentIO       = winrt.GUID("6cd0e74e-ee65-4489-9ebf-ca43e87ba637")
)

// Register 登记 AppUserModelID，并把图标写入 dir。重复调用只覆盖相同内容。
func Register(dir string) error {
	icon := filepath.Join(dir, "toast-icon.png")
	if b, err := iconPNG(); err == nil {
		if old, readErr := os.ReadFile(icon); readErr != nil || !bytes.Equal(old, b) {
			_ = os.MkdirAll(dir, 0700)
			_ = os.WriteFile(icon, b, 0600)
		}
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+AppID, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("登记通知身份失败：%w", err)
	}
	defer k.Close()
	if err := k.SetStringValue("DisplayName", "TapDeck"); err != nil {
		return err
	}
	if _, err := os.Stat(icon); err == nil {
		_ = k.SetStringValue("IconUri", icon)
	}
	return nil
}

// iconPNG 绘制与托盘一致的图标。
func iconPNG() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			v := color.RGBA{23, 92, 211, 255}
			if x > 15 && x < 49 && y > 15 && y < 49 {
				v = color.RGBA{255, 255, 255, 255}
			}
			img.Set(x, y, v)
		}
	}
	var b bytes.Buffer
	err := png.Encode(&b, img)
	return b.Bytes(), err
}

// Show 以 TapDeck 身份弹出一条 Toast；接收端的通知监听器会照常读到并转发。
func Show(m Message) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := winrt.Init(); err != nil {
		return err
	}
	inspectable, err := winrt.Activate("Windows.Data.Xml.Dom.XmlDocument")
	if err != nil {
		return err
	}
	defer inspectable.Release()
	io, err := inspectable.Query(&iidXmlDocumentIO)
	if err != nil {
		return err
	}
	defer io.Release()
	xml, err := winrt.NewString(ToastXML(m))
	if err != nil {
		return err
	}
	defer xml.Delete()
	if err := winrt.Check(io.Call(6, uintptr(xml))); err != nil { // LoadXml
		return fmt.Errorf("Toast 内容无效：%w", err)
	}
	doc, err := inspectable.Query(&iidXmlDocument)
	if err != nil {
		return err
	}
	defer doc.Release()
	factory, err := winrt.Factory("Windows.UI.Notifications.ToastNotification", &iidToastFactory)
	if err != nil {
		return err
	}
	defer factory.Release()
	toast, err := factory.Get(6, uintptr(unsafe.Pointer(doc))) // CreateToastNotification
	if err != nil {
		return err
	}
	defer toast.Release()
	manager, err := winrt.Factory("Windows.UI.Notifications.ToastNotificationManager", &iidToastManagerStatics)
	if err != nil {
		return err
	}
	defer manager.Release()
	id, err := winrt.NewString(AppID)
	if err != nil {
		return err
	}
	defer id.Delete()
	notifier, err := manager.Get(7, uintptr(id)) // CreateToastNotifier(String)
	if err != nil {
		return err
	}
	defer notifier.Release()
	if err := winrt.Check(notifier.Call(6, uintptr(unsafe.Pointer(toast)))); err != nil { // Show
		return fmt.Errorf("弹出通知失败：%w", err)
	}
	// 给通知平台片刻时间接收，进程随后即可退出。
	time.Sleep(200 * time.Millisecond)
	return nil
}
