//go:build windows

// Package apkdist 把编译好的 Android 安装包打进接收端，让配对网页可以直接扫码下载。
//
// 构建前由 scripts/build-windows.ps1 把 dist\TapDeck-debug.apk 复制成
// assets\TapDeck.apk；assets 目录里始终保留 README.txt，因此没有 APK 时也能编译
// （此时网页只显示 GitHub Release 链接）。
package apkdist

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"strings"
	"sync"
)

//go:embed assets
var assets embed.FS

var (
	once sync.Once
	name string
	data []byte
	sum  string
)

func load() {
	once.Do(func() {
		name, data, sum = scan(assets)
	})
}

// scan 找出目录里第一个 *.apk；没有则返回空值。
func scan(fsys fs.FS) (string, []byte, string) {
	entries, err := fs.ReadDir(fsys, "assets")
	if err != nil {
		return "", nil, ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".apk") {
			continue
		}
		b, err := fs.ReadFile(fsys, "assets/"+e.Name())
		if err != nil || len(b) == 0 {
			continue
		}
		h := sha256.Sum256(b)
		return e.Name(), b, hex.EncodeToString(h[:])
	}
	return "", nil, ""
}

// Available 表示本次构建是否内嵌了 APK。
func Available() bool {
	load()
	return len(data) > 0
}

// Name 返回内嵌 APK 的文件名（也是下载时的建议文件名）；未内嵌时为空。
func Name() string {
	load()
	return name
}

// Bytes 返回内嵌的 APK 内容；未内嵌时为 nil。
func Bytes() []byte {
	load()
	return data
}

// SHA256 返回内嵌 APK 的 SHA-256（小写十六进制）；未内嵌时为空。
func SHA256() string {
	load()
	return sum
}

// SizeText 返回便于显示的大小，例如 "11.0 MB"；未内嵌时为空。
func SizeText() string {
	load()
	if len(data) == 0 {
		return ""
	}
	const unit = 1024 * 1024
	if len(data) < unit {
		return fmt.Sprintf("%.0f KB", float64(len(data))/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(len(data))/unit)
}
