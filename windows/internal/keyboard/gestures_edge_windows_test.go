//go:build windows

package keyboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/coder/websocket"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"tapdeck/internal/input"
)

// Launch a separate Edge profile with remote debugging enabled. Never connect
// this acceptance fixture to the user's normal browser profile.
func TestNativeEdgePageAndImageZoom(t *testing.T) {
	nativeOptIn(t)
	port := os.Getenv("TAPDECK_NATIVE_EDGE_ZOOM")
	if port == "" {
		t.Skip("requires isolated Edge profile debugging port")
	}
	response, err := http.Get("http://127.0.0.1:" + port + "/json/list")
	if err != nil {
		t.Fatal(err)
	}
	var targets []struct{ ID, Title, WebSocketDebuggerURL string }
	err = json.NewDecoder(response.Body).Decode(&targets)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	var targetID, endpoint string
	for _, target := range targets {
		if target.Title == "TapDeckGestureZoomValidation" {
			targetID, endpoint = target.ID, target.WebSocketDebuggerURL
			break
		}
	}
	if endpoint == "" {
		t.Fatal("isolated zoom fixture not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		data, _ := json.Marshal(map[string]any{"id": 100000, "method": "Browser.close"})
		_ = connection.Write(cleanup, websocket.MessageText, data)
	}()
	var id int
	rpc := func(method string, params any) json.RawMessage {
		t.Helper()
		id++
		data, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
		if err := connection.Write(ctx, websocket.MessageText, data); err != nil {
			t.Fatal(err)
		}
		for {
			_, data, err := connection.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var answer struct {
				ID     int
				Result json.RawMessage
				Error  any
			}
			if json.Unmarshal(data, &answer) != nil || answer.ID != id {
				continue
			}
			if answer.Error != nil {
				t.Fatalf("CDP %s: %v", method, answer.Error)
			}
			return answer.Result
		}
	}
	evaluate := func() float64 {
		t.Helper()
		data := rpc("Runtime.evaluate", map[string]any{"expression": "window.devicePixelRatio", "returnByValue": true})
		var result struct{ Result struct{ Value float64 } }
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result.Result.Value
	}
	foreground := win.GetForegroundWindow()
	var cursor win.POINT
	win.GetCursorPos(&cursor)
	t.Cleanup(func() { win.SetForegroundWindow(foreground); win.SetCursorPos(cursor.X, cursor.Y) })
	rpc("Target.activateTarget", map[string]any{"targetId": targetID})
	var browserWindow struct{ WindowID int }
	if err := json.Unmarshal(rpc("Browser.getWindowForTarget", map[string]any{"targetId": targetID}), &browserWindow); err != nil {
		t.Fatal(err)
	}
	rpc("Browser.setWindowBounds", map[string]any{"windowId": browserWindow.WindowID, "bounds": map[string]any{"windowState": "normal"}})
	time.Sleep(200 * time.Millisecond)
	var hwnd win.HWND
	callback := syscall.NewCallback(func(window win.HWND, _ uintptr) uintptr {
		var text [256]uint16
		n, _, _ := windowUser32.NewProc("GetWindowTextW").Call(uintptr(window), uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)))
		if n > 0 && strings.Contains(windows.UTF16ToString(text[:n]), "TapDeckGestureZoomValidation") && windowClass(uintptr(window)) == "Chrome_WidgetWin_1" {
			hwnd = window
			return 0
		}
		return 1
	})
	windowUser32.NewProc("EnumWindows").Call(callback, 0)
	if hwnd == 0 {
		t.Fatal("isolated Edge window missing")
	}
	rpc("Target.activateTarget", map[string]any{"targetId": targetID})
	focusGestureWindow(t, &gestureTestWindow{hwnd: hwnd})
	soft := input.New()
	t.Cleanup(soft.ReleaseAll)
	verify := func(label string) {
		t.Helper()
		before := evaluate()
		if err := soft.ZoomSteps(1, false); err != nil {
			t.Fatal(err)
		}
		nativeAwait(t, label+" did not enlarge", func() bool { return evaluate() > before*1.05 })
		after := evaluate()
		if err := soft.ZoomSteps(-1, false); err != nil {
			t.Fatal(err)
		}
		nativeAwait(t, label+" did not restore", func() bool { return evaluate() == before })
		if input.CurrentModifiers().Ctrl {
			t.Fatal("Ctrl held after Edge zoom")
		}
		t.Logf("%s devicePixelRatio: %.2f -> %.2f -> %.2f", label, before, after, evaluate())
	}
	verify("Edge webpage")
	image := os.Getenv("TAPDECK_NATIVE_EDGE_IMAGE")
	if image == "" {
		image = "../../../dist/0.3.3/screenshots/phone-100-keyboard.png"
	}
	image, err = filepath.Abs(image)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(image); err != nil {
		t.Fatal(err)
	}
	address := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(image)}).String()
	rpc("Page.navigate", map[string]any{"url": address})
	time.Sleep(500 * time.Millisecond)
	verify("Edge image viewer")
	// Browser.close affects only the deliberately isolated debugging profile.
	data, _ := json.Marshal(map[string]any{"id": id + 1, "method": "Browser.close"})
	_ = connection.Write(ctx, websocket.MessageText, data)
	t.Log(fmt.Sprintf("isolated Edge acceptance on port %s completed", port))
}
