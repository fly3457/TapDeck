package notify

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Message 是要以 Toast 弹出的一条提醒。
type Message struct{ Title, Body string }

// SummaryLimit 是 Agent 最后回复摘要的字符数。
const SummaryLimit = 120

// transcriptTail 是读取 Claude Code 会话记录尾部的字节数，足以找到最后一条回复。
const transcriptTail = 512 << 10

// ParseClaudeHook 解析 Claude Code hook 从 stdin 传入的 JSON。Stop / SubagentStop
// 生成“任务完成”提醒并尽量附上最后一条回复的摘要；Notification 使用其 message。
func ParseClaudeHook(payload []byte) (Message, error) {
	var p struct {
		Event          string `json:"hook_event_name"`
		Cwd            string `json:"cwd"`
		Message        string `json:"message"`
		Title          string `json:"title"`
		TranscriptPath string `json:"transcript_path"`
		LastAssistant  string `json:"last_assistant_message"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(payload), &p); err != nil {
		return Message{}, fmt.Errorf("无法解析 Claude Code hook 数据：%w", err)
	}
	title := agentTitle("Claude Code", p.Cwd)
	switch p.Event {
	case "Notification":
		body := strings.TrimSpace(p.Message)
		if body == "" {
			body = "需要你的处理"
		}
		return Message{title, body}, nil
	case "Stop", "SubagentStop", "":
		summary := strings.TrimSpace(p.LastAssistant)
		if summary == "" && p.TranscriptPath != "" {
			summary = lastClaudeReply(p.TranscriptPath)
		}
		body := "任务完成"
		if summary != "" {
			body = "任务完成：" + Clip(oneLine(summary), SummaryLimit)
		}
		return Message{title, body}, nil
	default:
		return Message{title, p.Event}, nil
	}
}

// ParseCodexHook 解析 Codex `notify` 程序收到的最后一个参数（JSON）。
func ParseCodexHook(arg string) (Message, error) {
	var p struct {
		Type          string `json:"type"`
		Cwd           string `json:"cwd"`
		LastAssistant string `json:"last-assistant-message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(arg)), &p); err != nil {
		return Message{}, fmt.Errorf("无法解析 Codex 通知数据：%w", err)
	}
	title := agentTitle("Codex", p.Cwd)
	if p.Type != "" && p.Type != "agent-turn-complete" {
		return Message{title, p.Type}, nil
	}
	body := "任务完成"
	if s := strings.TrimSpace(p.LastAssistant); s != "" {
		body = "任务完成：" + Clip(oneLine(s), SummaryLimit)
	}
	return Message{title, body}, nil
}

func agentTitle(agent, cwd string) string {
	cwd = strings.TrimRight(strings.TrimSpace(cwd), `\/`)
	if cwd == "" {
		return agent
	}
	return agent + " · " + filepath.Base(filepath.FromSlash(cwd))
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// lastClaudeReply 从会话记录（JSONL）尾部倒序找最后一条带文字的助手消息。
func lastClaudeReply(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > transcriptTail {
		_, _ = f.Seek(st.Size()-transcriptTail, io.SeekStart)
	}
	var lines [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), transcriptTail)
	for sc.Scan() {
		lines = append(lines, append([]byte(nil), sc.Bytes()...))
	}
	for i := len(lines) - 1; i >= 0; i-- {
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(lines[i], &entry) != nil || entry.Type != "assistant" {
			continue
		}
		var parts []struct{ Type, Text string }
		if json.Unmarshal(entry.Message.Content, &parts) != nil {
			continue
		}
		var text []string
		for _, part := range parts {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				text = append(text, part.Text)
			}
		}
		if len(text) > 0 {
			return strings.Join(text, " ")
		}
	}
	return ""
}

// ToastXML 生成通用 Toast 模板。
func ToastXML(m Message) string {
	var b strings.Builder
	b.WriteString(`<toast><visual><binding template="ToastGeneric"><text>`)
	b.WriteString(escape(Clip(m.Title, MaxTitle)))
	b.WriteString(`</text>`)
	if m.Body != "" {
		b.WriteString(`<text>`)
		b.WriteString(escape(Clip(m.Body, MaxBody)))
		b.WriteString(`</text>`)
	}
	b.WriteString(`</binding></visual></toast>`)
	return b.String()
}

func escape(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		// XML 1.0 不允许的控制字符直接丢弃，否则 LoadXml 会失败。
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			continue
		}
		switch r {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
