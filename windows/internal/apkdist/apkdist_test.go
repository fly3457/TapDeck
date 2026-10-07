//go:build windows

package apkdist

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"testing/fstest"
)

func TestScanFindsAPK(t *testing.T) {
	body := []byte("PK\x03\x04 fake apk")
	fsys := fstest.MapFS{
		"assets/README.txt": {Data: []byte("说明")},
		"assets/TapDeck.apk": {
			Data: body,
		},
	}
	name, data, sum := scan(fsys)
	if name != "TapDeck.apk" {
		t.Fatalf("name = %q", name)
	}
	if string(data) != string(body) {
		t.Fatalf("data mismatch: %q", data)
	}
	want := sha256.Sum256(body)
	if sum != hex.EncodeToString(want[:]) {
		t.Fatalf("sha = %q", sum)
	}
}

// 没有 APK（例如只构建 Windows 端）时必须是「空」而不是报错。
func TestScanWithoutAPK(t *testing.T) {
	fsys := fstest.MapFS{"assets/README.txt": {Data: []byte("说明")}}
	name, data, sum := scan(fsys)
	if name != "" || data != nil || sum != "" {
		t.Fatalf("expected empty result, got %q %v %q", name, data, sum)
	}
	// 连 assets 目录都不存在时也不能 panic。
	if name, data, sum := scan(fstest.MapFS{}); name != "" || data != nil || sum != "" {
		t.Fatalf("missing assets directory: %q %v %q", name, data, sum)
	}
}

// 空文件（例如中断的复制）不能当成有效 APK。
func TestScanIgnoresEmptyAPK(t *testing.T) {
	fsys := fstest.MapFS{"assets/TapDeck.apk": {Data: nil}}
	if name, data, _ := scan(fsys); name != "" || data != nil {
		t.Fatalf("empty apk accepted: %q %v", name, data)
	}
}
