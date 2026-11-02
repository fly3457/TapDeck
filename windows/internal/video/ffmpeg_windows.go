//go:build windows

package video

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed assets
var assets embed.FS

// Manifest 记录内嵌 ffmpeg 的来源、构建参数与哈希，由 scripts/build-ffmpeg.ps1 生成。
type Manifest struct {
	Version   string `json:"version"`
	Source    string `json:"source"`
	SHA256    string `json:"sha256"`
	Bytes     int64  `json:"bytes"`
	Configure string `json:"configure"`
}

// FFmpeg 是基于内嵌 ffmpeg 子进程的 Launcher。
type FFmpeg struct {
	once    sync.Once
	path    string
	err     error
	probeMu sync.Mutex
	probed  map[string]string
}

func NewLauncher() *FFmpeg { return &FFmpeg{probed: map[string]string{}} }

// EmbeddedManifest 返回内嵌 ffmpeg 的清单；没有内嵌时返回错误。
func EmbeddedManifest() (Manifest, []byte, error) {
	var m Manifest
	b, err := assets.ReadFile("assets/manifest.json")
	if err != nil {
		return m, nil, errors.New("接收端未内嵌 ffmpeg")
	}
	if err = json.Unmarshal(b, &m); err != nil {
		return m, nil, err
	}
	exe, err := assets.ReadFile("assets/ffmpeg.exe")
	if err != nil {
		return m, nil, errors.New("接收端未内嵌 ffmpeg")
	}
	sum := sha256.Sum256(exe)
	if hex.EncodeToString(sum[:]) != m.SHA256 || int64(len(exe)) != m.Bytes {
		return m, nil, errors.New("内嵌 ffmpeg 与清单不一致")
	}
	return m, exe, nil
}

func (f *FFmpeg) binary() (string, error) {
	f.once.Do(func() {
		// 诊断用：指定外部 ffmpeg 以便在未内嵌时测试。
		if p := os.Getenv("TAPDECK_FFMPEG"); p != "" {
			if _, err := os.Stat(p); err == nil {
				f.path = p
				return
			}
		}
		m, exe, err := EmbeddedManifest()
		if err != nil {
			f.err = err
			return
		}
		dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "TapDeck", "ffmpeg", m.SHA256[:16])
		path := filepath.Join(dir, "ffmpeg.exe")
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, exe) {
			f.path = path
			return
		}
		if err = os.MkdirAll(dir, 0o755); err != nil {
			f.err = err
			return
		}
		tmp := path + ".tmp"
		if err = os.WriteFile(tmp, exe, 0o755); err == nil {
			err = os.Rename(tmp, path)
		}
		if err != nil {
			_ = os.Remove(tmp)
			f.err = fmt.Errorf("释放 ffmpeg 失败：%w", err)
			return
		}
		f.path = path
	})
	return f.path, f.err
}

func (f *FFmpeg) Available() bool { _, err := f.binary(); return err == nil }

func (f *FFmpeg) Cursor(d Display) (int, int, bool) { return CursorOn(d) }

// Prepare 定位主显示器，首次按优先级探测可用编码器并缓存结果。
func (f *FFmpeg) Prepare(r Request) (Plan, error) {
	bin, err := f.binary()
	if err != nil {
		return Plan{}, err
	}
	d, err := PrimaryDisplay()
	if err != nil {
		return Plan{}, err
	}
	base, err := NewPlan(d, r, "")
	if err != nil {
		return Plan{}, err
	}
	key := fmt.Sprintf("%d:%d:%dx%d:%dx%d", d.Adapter, d.Output, d.Width, d.Height, base.Width, base.Height)
	f.probeMu.Lock()
	defer f.probeMu.Unlock()
	if enc, ok := f.probed[key]; ok {
		base.Encoder = enc
		return base, nil
	}
	var failures []string
	for _, enc := range Encoders {
		p := base
		p.Encoder = enc
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := command(ctx, bin, p.Args(true))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		cancel()
		if err == nil {
			log.Printf("video: 使用编码器 %s（%dx%d → %dx%d@%d）", enc, d.Width, d.Height, p.Width, p.Height, p.FPS)
			f.probed[key] = enc
			return p, nil
		}
		failures = append(failures, enc+": "+lastLine(stderr.String(), err))
	}
	log.Printf("video: 没有可用编码器：%s", strings.Join(failures, "；"))
	return Plan{}, ErrNoEncoder
}

type process struct {
	cmd    *exec.Cmd
	reader *flvReader
	stderr *tail
	once   sync.Once
}

func (f *FFmpeg) Launch(ctx context.Context, p Plan) (Source, error) {
	bin, err := f.binary()
	if err != nil {
		return nil, err
	}
	cmd := command(ctx, bin, p.Args(false))
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	t := &tail{}
	cmd.Stderr = t
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("无法启动编码进程：%w", err)
	}
	assignJob(cmd.Process.Pid)
	return &process{cmd: cmd, reader: newFLVReader(out), stderr: t}, nil
}

func (p *process) Next() (Frame, error) {
	f, err := p.reader.Next()
	if err != nil {
		return Frame{}, fmt.Errorf("编码进程结束：%s", lastLine(p.stderr.String(), err))
	}
	return f, nil
}

func (p *process) Close() error {
	p.once.Do(func() {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	})
	return nil
}

func command(ctx context.Context, bin string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}

// tail 保留 ffmpeg 最后 4KB 的诊断输出。
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > 4096 {
		t.buf = t.buf[len(t.buf)-4096:]
	}
	return len(b), nil
}
func (t *tail) String() string { t.mu.Lock(); defer t.mu.Unlock(); return string(t.buf) }

func lastLine(s string, err error) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	if err == io.EOF {
		return "输出结束"
	}
	return err.Error()
}

var (
	jobOnce sync.Once
	job     windows.Handle
)

// assignJob 把编码进程放进“随句柄关闭而终止”的作业对象：接收端崩溃或被结束时
// ffmpeg 一并退出，不会继续占用 GPU 截屏。
func assignJob(pid int) {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err = windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(h)
			return
		}
		job = h
	})
	if job == 0 {
		return
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(proc)
	_ = windows.AssignProcessToJobObject(job, proc)
}
