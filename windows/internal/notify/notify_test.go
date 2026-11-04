package notify

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDifferPrimesBaselineThenReportsNewIDs(t *testing.T) {
	var d Differ
	if got := d.Next([]uint32{1, 2, 3}); len(got) != 0 {
		t.Fatalf("baseline reported %v", got)
	}
	if got := d.Next([]uint32{2, 3, 4, 5}); !reflect.DeepEqual(got, []uint32{4, 5}) {
		t.Fatalf("fresh = %v", got)
	}
	// 被清除的通知不再出现，也不会被误报。
	if got := d.Next([]uint32{5}); len(got) != 0 {
		t.Fatalf("after dismiss = %v", got)
	}
	if got := d.Next(nil); len(got) != 0 {
		t.Fatalf("empty = %v", got)
	}
	if got := d.Next([]uint32{6}); !reflect.DeepEqual(got, []uint32{6}) {
		t.Fatalf("after empty = %v", got)
	}
}

func TestRingAssignsSeqAndReplays(t *testing.T) {
	r := NewRing(3)
	for i := 0; i < 5; i++ {
		if n := r.Add(Notification{Title: string(rune('a' + i))}); n.Seq != uint64(i+1) {
			t.Fatalf("seq = %d", n.Seq)
		}
	}
	all := r.Since("other-boot", 4)
	if len(all) != 3 || all[0].Seq != 3 || all[2].Seq != 5 {
		t.Fatalf("different boot must replay whole buffer: %+v", all)
	}
	if got := r.Since(r.Boot(), 4); len(got) != 1 || got[0].Seq != 5 {
		t.Fatalf("since 4 = %+v", got)
	}
	if got := r.Since(r.Boot(), 5); len(got) != 0 {
		t.Fatalf("up to date = %+v", got)
	}
	if NewRing(1).Boot() == r.Boot() {
		t.Fatal("boot ids must differ")
	}
}

func TestNormalizeClipsAndFilters(t *testing.T) {
	n := Notification{App: " 微信 ", Title: " 张三 ", Body: strings.Repeat("字", MaxBody+10)}.Normalize()
	if n.App != "微信" || n.Title != "张三" || len([]rune(n.Body)) != MaxBody || !strings.HasSuffix(n.Body, "…") {
		t.Fatalf("normalize = %+v", n)
	}
	if !(Notification{}).Empty() || (Notification{Body: "x"}).Empty() {
		t.Fatal("empty check")
	}
	list := ParseList("微信, Google Chrome；\nOutlook、 ,")
	if !reflect.DeepEqual(list, []string{"微信", "Google Chrome", "Outlook"}) {
		t.Fatalf("list = %v", list)
	}
	if !Excluded("google chrome", list) || Excluded("Slack", list) {
		t.Fatal("exclude match")
	}
}

func TestClaudeHookPayloads(t *testing.T) {
	m, err := ParseClaudeHook([]byte(`{"hook_event_name":"Notification","cwd":"D:\\project\\TapDeck","message":"Claude needs your permission to use Bash"}`))
	if err != nil || m.Title != "Claude Code · TapDeck" || m.Body != "Claude needs your permission to use Bash" {
		t.Fatalf("notification = %+v %v", m, err)
	}
	dir := t.TempDir()
	transcript := filepath.Join(dir, "t.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"做点事"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"旧回复"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"已完成通知转发，\n测试全部通过。"},{"type":"tool_use","name":"Bash"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read"}]}}`,
		`{"type":"system","subtype":"stop"}`,
	}
	if err := os.WriteFile(transcript, []byte(strings.Join(lines, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	m, err = ParseClaudeHook([]byte(`{"hook_event_name":"Stop","cwd":"/d/project/TapDeck/","transcript_path":` + quote(transcript) + `}`))
	if err != nil || m.Title != "Claude Code · TapDeck" || m.Body != "任务完成：已完成通知转发， 测试全部通过。" {
		t.Fatalf("stop = %+v %v", m, err)
	}
	m, err = ParseClaudeHook([]byte(`{"hook_event_name":"Stop","transcript_path":"missing.jsonl"}`))
	if err != nil || m.Title != "Claude Code" || m.Body != "任务完成" {
		t.Fatalf("stop fallback = %+v %v", m, err)
	}
	if _, err := ParseClaudeHook([]byte("not json")); err == nil {
		t.Fatal("bad json accepted")
	}
}

func TestCodexHookPayload(t *testing.T) {
	m, err := ParseCodexHook(`{"type":"agent-turn-complete","cwd":"C:\\work\\api","last-assistant-message":"` + strings.Repeat("a", 200) + `"}`)
	if err != nil || m.Title != "Codex · api" || len([]rune(m.Body)) != len([]rune("任务完成："))+SummaryLimit {
		t.Fatalf("codex = %+v %v", m, err)
	}
}

func TestToastXMLEscapes(t *testing.T) {
	got := ToastXML(Message{Title: `a<b>&"c'`, Body: "x\x01y"})
	want := `<toast><visual><binding template="ToastGeneric"><text>a&lt;b&gt;&amp;&quot;c&apos;</text><text>xy</text></binding></visual></toast>`
	if got != want {
		t.Fatalf("xml = %s", got)
	}
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"` }

func TestAppNameFromID(t *testing.T) {
	for in, want := range map[string]string{
		AppID: "TapDeck", "": "", `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`: "powershell",
		"Microsoft.Windows.Explorer": "Explorer", "Chrome": "Chrome",
	} {
		if got := appNameFromID(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}
