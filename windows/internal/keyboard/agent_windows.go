//go:build windows

package keyboard

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"tapdeck/internal/hidkeyboard"
	"tapdeck/internal/input"
)

type Agent struct {
	mu        sync.Mutex
	writeMu   sync.Mutex
	cmd       *exec.Cmd
	pipe      io.WriteCloser
	encoder   *json.Encoder
	epoch, id uint64
	waiters   map[uint64]func(error)
	busy      map[uint64]bool
	voices    map[string]bool
	held      int
	status    Status
	failed    bool
	closed    bool
	changing  bool
	softKeys  []uint16
	mouse     *input.Controller
}

func NewAgent(mode string) (*Agent, error) {
	a := &Agent{epoch: 1, waiters: map[uint64]func(error){}, busy: map[uint64]bool{}, voices: map[string]bool{}, mouse: input.New()}
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	a.cmd = exec.Command(path, "-keyboard-worker")
	a.cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	a.cmd.Stderr = os.Stderr
	in, err := a.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := a.cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	a.pipe, a.encoder = in, json.NewEncoder(in)
	if err = a.cmd.Start(); err != nil {
		in.Close()
		out.Close()
		return nil, err
	}
	ready := make(chan struct{})
	go func() {
		dec := json.NewDecoder(out)
		first := true
		for {
			var r response
			if err := dec.Decode(&r); err != nil {
				if first {
					close(ready)
				}
				a.fail(fmt.Errorf("键盘工作进程已断开: %w", err))
				return
			}
			a.mu.Lock()
			a.status = r.Status
			a.softKeys = r.SoftKeys
			done := a.waiters[r.ID]
			delete(a.waiters, r.ID)
			delete(a.busy, r.ID)
			a.mu.Unlock()
			if first {
				close(ready)
				first = false
			}
			if done != nil {
				var er error
				if r.Error != "" {
					er = fmt.Errorf("%s", r.Error)
				}
				go done(er)
			}
		}
	}()
	go func() { _ = a.cmd.Wait() }()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		a.Close()
		return nil, fmt.Errorf("键盘工作进程启动超时")
	}
	if err = a.ConfigureBackend(mode); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}
func (a *Agent) fail(err error) {
	a.mu.Lock()
	if a.failed || a.closed {
		a.mu.Unlock()
		return
	}
	a.failed = true
	a.status.Ready = false
	a.status.Error = err.Error()
	waiters := a.waiters
	a.waiters = map[uint64]func(error){}
	a.busy = map[uint64]bool{}
	a.voices = map[string]bool{}
	a.held = 0
	soft := append([]uint16(nil), a.softKeys...)
	a.softKeys = nil
	a.mu.Unlock()
	// If the child itself crashes, clear the device's keyboard report. Parent
	// death follows the separate pipe-EOF cleanup path inside the child.
	if d, e := hidkeyboard.Open(); e == nil {
		_ = d.Update(nil)
		d.Close()
	}
	input.ReleaseOwnedKeys(soft)
	for _, done := range waiters {
		go done(err)
	}
	log.Print(err)
}
func (a *Agent) submit(r request, done func(error)) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	a.mu.Lock()
	if a.failed || a.closed {
		a.mu.Unlock()
		return fmt.Errorf("键盘工作进程不可用，请重启 TapDeck")
	}
	if a.changing && r.Action != "configure" && r.Action != "clear" && r.Action != "voice_stop" {
		a.mu.Unlock()
		return fmt.Errorf("键盘发送方式正在切换")
	}
	a.id++
	r.ID, r.Epoch = a.id, a.epoch
	if len(a.waiters) >= 128 && r.Action != "clear" && r.Action != "voice_stop" {
		a.mu.Unlock()
		return fmt.Errorf("键盘队列已满")
	}
	if done == nil {
		done = func(error) {}
	}
	a.waiters[r.ID] = done
	if r.Action != "status" {
		a.busy[r.ID] = true
	}
	a.mu.Unlock()
	if err := a.encoder.Encode(r); err != nil {
		a.fail(err)
		return err
	}
	return nil
}
func (a *Agent) sync(r request) error {
	done := make(chan error, 1)
	if err := a.submit(r, func(err error) { done <- err }); err != nil {
		return err
	}
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		a.fail(fmt.Errorf("键盘工作进程无响应"))
		return fmt.Errorf("键盘动作超时")
	}
}
func (a *Agent) KeyboardStatus() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.status
	s.Busy = a.changing || len(a.busy) > 0 || len(a.voices) > 0 || a.held > 0
	return s
}
func (a *Agent) ConfigureBackend(mode string) error {
	a.mu.Lock()
	busy := a.changing || len(a.busy) > 0 || len(a.voices) > 0 || a.held > 0
	if !busy {
		a.changing = true
	}
	a.mu.Unlock()
	if busy {
		return fmt.Errorf("录音或按键执行期间不能切换键盘发送方式")
	}
	defer func() { a.mu.Lock(); a.changing = false; a.mu.Unlock() }()
	return a.sync(request{Action: "configure", Mode: normalize(mode)})
}
func (a *Agent) ChordAsync(k string, done func(error)) error {
	if err := Validate(k, a.KeyboardStatus().Configured); err != nil {
		return err
	}
	return a.submit(request{Action: "chord", Chord: k}, done)
}
func (a *Agent) Chord(k string) error { return a.sync(request{Action: "chord", Chord: k}) }

// HoldAsync presses (down) or releases a shortcut while the phone keeps the
// button pressed, so holding the button repeats the key state instead of firing
// once. The held count keeps a held shortcut from being released by unrelated
// work such as a voice hotkey transition.
func (a *Agent) HoldAsync(k string, down bool, done func(error)) error {
	if err := Validate(k, a.KeyboardStatus().Configured); err != nil {
		return err
	}
	action := "hold_async"
	if !down {
		action = "hold_up"
	}
	err := a.submit(request{Action: action, Chord: k}, func(err error) {
		if err == nil {
			a.mu.Lock()
			if down {
				a.held++
			} else if a.held > 0 {
				a.held--
			}
			a.mu.Unlock()
		}
		done(err)
	})
	return err
}
func (a *Agent) Hold(k string, down bool) error {
	err := a.sync(request{Action: "hold", Chord: k, Down: down})
	if err == nil {
		a.mu.Lock()
		if down {
			a.held++
		} else if a.held > 0 {
			a.held--
		}
		a.mu.Unlock()
	}
	return err
}
func (a *Agent) StartVoice(token, mode, start, stop string, done func(error)) error {
	if err := Validate(start, a.KeyboardStatus().Configured); err != nil {
		return err
	}
	if err := Validate(stop, a.KeyboardStatus().Configured); err != nil {
		return err
	}
	a.mu.Lock()
	a.voices[token] = true
	a.mu.Unlock()
	err := a.submit(request{Action: "voice_start", Token: token, Mode: mode, Chord: start, Stop: stop}, func(err error) {
		if err != nil {
			a.mu.Lock()
			delete(a.voices, token)
			a.mu.Unlock()
		}
		done(err)
	})
	if err != nil {
		a.mu.Lock()
		delete(a.voices, token)
		a.mu.Unlock()
	}
	return err
}
func (a *Agent) StopVoice(token string, done func(error)) error {
	return a.submit(request{Action: "voice_stop", Token: token}, func(err error) { a.mu.Lock(); delete(a.voices, token); a.mu.Unlock(); done(err) })
}
func (a *Agent) Move(dx, dy, sx, sy int32) error  { return a.mouse.Move(dx, dy, sx, sy) }
func (a *Agent) Button(b string, down bool) error { return a.mouse.Button(b, down) }
func (a *Agent) ReleaseAll() {
	a.mouse.ReleaseAll()
	a.mu.Lock()
	a.epoch++
	a.voices = map[string]bool{}
	a.held = 0
	a.mu.Unlock()
	if err := a.submit(request{Action: "clear"}, func(err error) {
		if err != nil {
			log.Printf("键盘清理: %v", err)
		}
	}); err != nil {
		log.Print(err)
	}
}
func (a *Agent) Close() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	a.mu.Unlock()
	a.mouse.ReleaseAll()
	// Closing the inherited pipe is also the graceful shutdown command: the
	// child cancels pending actions, ends toggle voice once and releases keys.
	a.writeMu.Lock()
	_ = a.pipe.Close()
	a.writeMu.Unlock()
}
