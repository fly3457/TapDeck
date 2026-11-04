package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
	"tapdeck/internal/config"
	"tapdeck/internal/notify"
	"tapdeck/internal/server"
)

// notifyAccess 是通知监听器最近一次报告的 Windows 通知访问权限，供设置页显示。
var notifyAccess atomic.Int32

// notifyShimPath 是 Agent hook 调用的稳定入口；每次启动接收端都指向当前 EXE，
// hook 配置不随版本文件名变化。
func notifyShimPath(dir string) string { return filepath.Join(dir, "bin", "tapdeck-notify.cmd") }

// runNotifyCLI 处理 --notify-title / --agent-hook：弹出一条 Toast 后退出，不启动接收端。
func runNotifyCLI(title, body, hook string, args []string) {
	m := notify.Message{Title: title, Body: body}
	switch hook {
	case "":
	case "claude":
		payload, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		if err != nil {
			log.Fatalf("读取 Claude Code hook 数据失败：%v", err)
		}
		if m, err = notify.ParseClaudeHook(payload); err != nil {
			log.Printf("%v；使用默认提醒", err)
			m = notify.Message{Title: "Claude Code", Body: "任务完成"}
		}
	case "codex":
		var err error
		m = notify.Message{Title: "Codex", Body: "任务完成"}
		if len(args) > 0 {
			if parsed, parseErr := notify.ParseCodexHook(args[len(args)-1]); parseErr == nil {
				m = parsed
			} else {
				err = parseErr
			}
		}
		if err != nil {
			log.Printf("%v；使用默认提醒", err)
		}
	default:
		log.Fatalf("未知的 --agent-hook：%s（支持 claude、codex）", hook)
	}
	if strings.TrimSpace(m.Title) == "" {
		m.Title = "TapDeck"
	}
	if err := notify.Register(config.Directory()); err != nil {
		log.Print(err)
	}
	if err := notify.Show(m); err != nil {
		log.Fatal(err)
	}
}

// startNotifications 登记 Toast 身份、写入 hook 入口，并开始监听 Windows 通知。
func startNotifications(ctx context.Context, s *server.Server, dir string) {
	if err := notify.Register(dir); err != nil {
		log.Print(err)
	}
	if err := writeNotifyShim(dir); err != nil {
		log.Printf("写入 Agent 通知入口失败：%v", err)
	}
	s.EnableNotifications()
	go func() {
		err := notify.Watch(ctx, time.Second, func(n notify.Notification) { s.PublishNotification(n) }, func(a notify.Access) {
			notifyAccess.Store(int32(a))
			log.Printf("Windows 通知访问：%s", a.Text())
		})
		if err != nil {
			log.Printf("通知监听停止：%v", err)
		}
	}()
}

func writeNotifyShim(dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path := notifyShimPath(dir)
	// cmd.exe 按系统代码页解析批处理，注释只用 ASCII，避免多字节字符吞掉换行。
	content := fmt.Sprintf("@echo off\r\nrem Rewritten by TapDeck on every start: Agent hooks show Windows notifications through the current receiver.\r\n\"%s\" %%*\r\n", exe)
	b := ansiBytes(content)
	if old, err := os.ReadFile(path); err == nil && string(old) == string(b) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return config.AtomicWrite(path, b)
}

var procWideCharToMultiByte = windows.NewLazySystemDLL("kernel32.dll").NewProc("WideCharToMultiByte")

// ansiBytes 把文本转换为系统 ANSI 代码页（cmd.exe 解析批处理所用），使含中文的 EXE 路径
// 在批处理中仍然正确；无法无损转换时退回 UTF-8。
func ansiBytes(s string) []byte {
	u := windows.StringToUTF16(s)
	n := len(u) - 1
	if n <= 0 {
		return []byte(s)
	}
	var lossy int32
	size, _, _ := procWideCharToMultiByte.Call(0, 0, uintptr(unsafe.Pointer(&u[0])), uintptr(n), 0, 0, 0, uintptr(unsafe.Pointer(&lossy))) // CP_ACP
	if size == 0 || lossy != 0 {
		return []byte(s)
	}
	out := make([]byte, size)
	if r, _, _ := procWideCharToMultiByte.Call(0, 0, uintptr(unsafe.Pointer(&u[0])), uintptr(n), uintptr(unsafe.Pointer(&out[0])), size, 0, 0); r == 0 {
		return []byte(s)
	}
	return out
}

// claudeHookConfig 生成可粘贴进 ~/.claude/settings.json 的 hooks 片段。
func claudeHookConfig(dir string) string {
	command := fmt.Sprintf("\"%s\" --agent-hook claude", filepath.ToSlash(notifyShimPath(dir)))
	entry := []map[string]any{{"hooks": []map[string]any{{"type": "command", "command": command}}}}
	b, _ := json.MarshalIndent(map[string]any{"hooks": map[string]any{"Stop": entry, "Notification": entry}}, "", "  ")
	return string(b)
}

// codexNotifyConfig 生成 ~/.codex/config.toml 的 notify 行。
func codexNotifyConfig(dir string) string {
	return fmt.Sprintf("notify = [\"%s\", \"--agent-hook\", \"codex\"]", filepath.ToSlash(notifyShimPath(dir)))
}

type notifySettings struct {
	enabled *walk.CheckBox
	status  *walk.Label
	exclude *walk.LineEdit
}

func notifySettingsPage(owner func() walk.Form, dir string, cfg config.Config, w *notifySettings) d.TabPage {
	copyText := func(text, what string) {
		if err := walk.Clipboard().SetText(text); err != nil {
			walk.MsgBox(owner(), "复制失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		walk.MsgBox(owner(), "已复制", what, walk.MsgBoxIconInformation)
	}
	return d.TabPage{Title: "通知", Layout: d.VBox{}, Children: []d.Widget{
		settingsSection("Windows 通知转发", 0,
			d.CheckBox{AssignTo: &w.enabled, Text: "把 Windows 系统通知转发到已连接的手机", Checked: !cfg.NotifyDisabled},
			d.Label{AssignTo: &w.status, Text: notify.AccessUnknown.Text()},
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.PushButton{Text: "打开 Windows 通知访问设置", OnClicked: func() { open("ms-settings:privacy-notifications") }},
				d.PushButton{Text: "发送测试通知", OnClicked: func() {
					go func() {
						if err := notify.Show(notify.Message{Title: "TapDeck 测试通知", Body: "手机收到这条通知即表示转发正常。"}); err != nil {
							log.Printf("测试通知失败：%v", err)
						}
					}()
				}},
				d.HSpacer{},
			}},
			d.Label{Text: "不转发的应用（名称与 Windows 通知中心一致，逗号分隔）"},
			d.LineEdit{AssignTo: &w.exclude, Text: strings.Join(cfg.NotifyExclude, ", ")},
			d.Label{Text: "手机在后台时由前台服务保持连接；请在手机“连接与设置”中开启“后台接收电脑通知”。"},
		),
		settingsSection("Agent 完成提醒", 0,
			d.Label{Text: "Claude 桌面版等自身会弹 Windows 通知的应用无需配置，通知会直接转发。\nClaude Code / Codex 命令行版可通过 hook 在任务完成或需要处理时弹出通知："},
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.PushButton{Text: "复制 Claude Code 钩子配置", OnClicked: func() {
					copyText(claudeHookConfig(dir), "已复制 hooks 配置。请合并到 %USERPROFILE%\\.claude\\settings.json 的顶层（已有 hooks 时合并 Stop 与 Notification 两项）。")
				}},
				d.PushButton{Text: "复制 Codex 配置", OnClicked: func() {
					copyText(codexNotifyConfig(dir), "已复制 notify 配置。请写入 %USERPROFILE%\\.codex\\config.toml 的顶层。")
				}},
				d.HSpacer{},
			}},
			d.Label{Text: "入口：" + notifyShimPath(dir)},
		),
		d.VSpacer{},
	}}
}

// apply 把通知页的编辑写入待保存的配置。
func (w *notifySettings) apply(c *config.Config) {
	c.NotifyDisabled = !w.enabled.Checked()
	c.NotifyExclude = notify.ParseList(w.exclude.Text())
}

func (w *notifySettings) refresh() {
	if w.status != nil {
		_ = w.status.SetText(notify.Access(notifyAccess.Load()).Text())
	}
}
