package main

import (
	"os"
	"strings"
	"testing"
)

func TestNotifyShimPointsAtExecutableInSystemCodePage(t *testing.T) {
	dir := t.TempDir()
	if err := writeNotifyShim(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(notifyShimPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	if !strings.Contains(string(b), "@echo off\r\n") || !strings.HasSuffix(string(b), "%*\r\n") || !strings.Contains(string(b), string(ansiBytes(exe))) {
		t.Fatalf("shim = %q", b)
	}
	if got := ansiBytes("中文路径"); len(got) != 8 {
		t.Fatalf("GBK/ACP bytes = % x", got)
	}
	if !strings.Contains(claudeHookConfig(dir), "--agent-hook claude") || !strings.Contains(codexNotifyConfig(dir), `"--agent-hook", "codex"`) {
		t.Fatal("hook config")
	}
}
