// Package notify 读取 Windows 系统通知并缓存最近若干条，供接收端转发到手机；
// 也负责以 TapDeck 身份弹出 Toast（Agent 完成提醒等）。
package notify

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"unicode/utf8"
)

// Notification 是转发给手机的一条通知。Time 为 Unix 毫秒，Seq 由 Ring 分配。
type Notification struct {
	Seq   uint64 `json:"seq"`
	App   string `json:"app"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Time  int64  `json:"time"`
}

// 字段长度上限（按字符计）：通知正文可能是整段聊天消息，手机只需要摘要。
const (
	MaxApp   = 64
	MaxTitle = 200
	MaxBody  = 500
)

// Normalize 去掉首尾空白并按上限截断各字段。
func (n Notification) Normalize() Notification {
	n.App = Clip(strings.TrimSpace(n.App), MaxApp)
	n.Title = Clip(strings.TrimSpace(n.Title), MaxTitle)
	n.Body = Clip(strings.TrimSpace(n.Body), MaxBody)
	return n
}

// Empty 表示没有任何可显示的文字。
func (n Notification) Empty() bool { return n.Title == "" && n.Body == "" }

// Clip 按字符截断，超出时以省略号结尾。
func Clip(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return string(r[:limit-1]) + "…"
}

// appNameFromID 由 AppUserModelId 推出可读的应用名：TapDeck 自身显示为 TapDeck，
// 其他取最后一段（去掉 .exe 等扩展名与路径）。
func appNameFromID(aumid string) string {
	aumid = strings.TrimSpace(aumid)
	if aumid == "" {
		return ""
	}
	if strings.EqualFold(aumid, AppID) {
		return "TapDeck"
	}
	name := aumid[strings.LastIndexAny(aumid, `\/!`)+1:]
	if i := strings.LastIndex(name, "."); i > 0 {
		if ext := strings.ToLower(name[i:]); ext == ".exe" || ext == ".app" {
			name = name[:i]
		} else {
			name = name[i+1:]
		}
	}
	return name
}

// Excluded 判断应用名是否在排除列表中（忽略大小写与首尾空白）。
func Excluded(app string, exclude []string) bool {
	app = strings.TrimSpace(app)
	for _, v := range exclude {
		if v = strings.TrimSpace(v); v != "" && strings.EqualFold(v, app) {
			return true
		}
	}
	return false
}

// ParseList 把设置界面中以逗号、顿号、分号或换行分隔的应用名整理成列表。
func ParseList(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool { return strings.ContainsRune(",，、;；\r\n", r) })
	out := []string{}
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// Ring 保存最近的通知，供手机重连后按 seq 补发。Boot 每次进程启动随机生成，
// 手机据此判断 seq 是否仍可比较。
type Ring struct {
	mu    sync.Mutex
	boot  string
	next  uint64
	limit int
	items []Notification
}

// NewRing 创建最多保留 limit 条的缓冲。
func NewRing(limit int) *Ring {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return &Ring{boot: hex.EncodeToString(b), next: 1, limit: max(1, limit)}
}

// Boot 返回本次启动的标识。
func (r *Ring) Boot() string { return r.boot }

// Add 分配 seq 并保存，返回带 seq 的通知。
func (r *Ring) Add(n Notification) Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	n.Seq = r.next
	r.next++
	r.items = append(r.items, n)
	if len(r.items) > r.limit {
		r.items = append([]Notification(nil), r.items[len(r.items)-r.limit:]...)
	}
	return n
}

// Since 返回手机尚未收到的通知：boot 不同则返回全部缓冲，否则返回 seq > after 的项。
func (r *Ring) Since(boot string, after uint64) []Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Notification{}
	for _, n := range r.items {
		if boot != r.boot || n.Seq > after {
			out = append(out, n)
		}
	}
	return out
}

// Differ 按通知 Id 找出新出现的通知。首次调用只记录基线，不把启动前已在
// 通知中心的旧通知当作新通知。
type Differ struct {
	seen   map[uint32]bool
	primed bool
}

// Next 传入当前通知中心的全部 Id，返回本轮新增的 Id（保持输入顺序）。
func (d *Differ) Next(ids []uint32) []uint32 {
	current := make(map[uint32]bool, len(ids))
	fresh := []uint32{}
	for _, id := range ids {
		current[id] = true
		if d.primed && !d.seen[id] {
			fresh = append(fresh, id)
		}
	}
	d.seen, d.primed = current, true
	return fresh
}

// AppID 是 TapDeck 弹出 Toast 使用的 AppUserModelID。未打包应用在
// HKCU\Software\Classes\AppUserModelId 下登记显示名与图标即可弹出通知，无需管理员。
const AppID = "Yuncii.TapDeck"
