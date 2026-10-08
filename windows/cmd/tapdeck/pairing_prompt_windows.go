package main

import (
	"fmt"
	"strings"
	"syscall"
	"tapdeck/internal/server"
	"time"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

// Enter the modal loop from a native message, after Walk drains its synchronized
// callback batch. Entering inside that batch can strand a queued snapshot and
// its uiPending guard until the dialog closes, preventing timeout processing.
const pairingDialogTimer = 0x544450

var pairingDialogRuns = map[win.HWND]func(){} // Only accessed on the GUI thread.
var pairingDialogTimerProc = syscall.NewCallback(func(hwnd win.HWND, _ uint32, id uintptr, _ uint32) uintptr {
	win.KillTimer(hwnd, id)
	run := pairingDialogRuns[hwnd]
	delete(pairingDialogRuns, hwnd)
	if run != nil {
		run()
	}
	return 0
})

type pairingPrompts struct {
	owner     *walk.MainWindow
	dialog    *walk.Dialog
	activeID  string
	handled   map[string]bool
	approve   func(string, bool)
	showOwner func()
	present   func(*walk.Dialog)
	closeLive func()
}

func (p *pairingPrompts) update(requests []server.Pending, now time.Time) {
	if p.handled == nil {
		p.handled = make(map[string]bool)
	}
	live := make(map[string]bool, len(requests))
	for _, request := range requests {
		live[request.ID] = now.Before(request.Expires)
	}
	if p.dialog != nil && !live[p.activeID] {
		p.closeLive()
	}
	for id := range p.handled {
		if !live[id] {
			delete(p.handled, id)
		}
	}
	if p.dialog != nil || !p.owner.Enabled() {
		return
	}
	for _, request := range requests {
		if !live[request.ID] || p.handled[request.ID] {
			continue
		}
		if err := p.show(request); err != nil {
			// Leave this request eligible for the next update if creation failed.
			return
		}
		p.handled[request.ID] = true
		return
	}
}

func pairingCodeLines(code string) string {
	groups := strings.Fields(code)
	if len(groups) == 8 {
		return strings.Join(groups[:4], " ") + "\n" + strings.Join(groups[4:], " ")
	}
	return code
}

func (p *pairingPrompts) show(request server.Pending) error {
	var dlg *walk.Dialog
	var reject *walk.PushButton
	resolved := false
	finish := func(allow bool) {
		if resolved {
			return
		}
		resolved = true
		p.approve(request.ID, allow)
		dlg.Close(walk.DlgCmdOK)
	}
	err := (d.Dialog{AssignTo: &dlg, Title: "手机请求连接", CancelButton: &reject,
		MinSize: d.Size{Width: 480}, Font: d.Font{Family: "Microsoft YaHei UI", PointSize: 10},
		Layout: d.VBox{Margins: d.Margins{Left: 24, Right: 24, Top: 18, Bottom: 18}, Spacing: 12}, Children: []d.Widget{
			d.TextLabel{Text: request.Name + " 请求连接", Font: d.Font{Family: "Microsoft YaHei UI", PointSize: 13, Bold: true}, MinSize: d.Size{Width: 400}},
			d.TextLabel{Text: "设备标识：" + request.DeviceID, MinSize: d.Size{Width: 400}},
			d.Label{Text: "请核对手机上的校验码，一致后允许连接。"},
			d.Label{Text: pairingCodeLines(request.Code), Font: d.Font{Family: "Consolas", PointSize: 20, Bold: true}, TextColor: walk.RGB(23, 92, 211)},
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.HSpacer{}, d.PushButton{AssignTo: &reject, Text: "拒绝", OnClicked: func() { finish(false) }},
				d.PushButton{Text: "允许连接", OnClicked: func() { finish(true) }},
			}},
		}}).Create(p.owner)
	if err != nil {
		return fmt.Errorf("创建配对确认框: %w", err)
	}
	p.dialog, p.activeID = dlg, request.ID
	p.closeLive = func() { resolved = true; dlg.Close(walk.DlgCmdCancel) }
	dlg.Closing().Attach(func(_ *bool, _ walk.CloseReason) {
		win.KillTimer(dlg.Handle(), pairingDialogTimer)
		delete(pairingDialogRuns, dlg.Handle())
		if !resolved {
			resolved = true
			p.approve(request.ID, false)
		}
		p.dialog, p.activeID, p.closeLive = nil, "", nil
	})
	if p.showOwner != nil {
		p.showOwner()
	}
	win.EnableWindow(p.owner.Handle(), false)
	if p.present != nil {
		p.present(dlg)
	}
	dlg.Starting().Attach(func() {
		if p.present == nil {
			dlg.Activate()
			_ = reject.SetFocus()
		}
	})
	pairingDialogRuns[dlg.Handle()] = func() {
		if p.dialog == dlg {
			dlg.Run()
		}
	}
	if win.SetTimer(dlg.Handle(), pairingDialogTimer, 1, pairingDialogTimerProc) == 0 {
		p.closeLive()
		return fmt.Errorf("无法启动配对确认框")
	}
	return nil
}
