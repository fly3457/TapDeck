//go:build windows

// Package apkdist 把编译好的 Android 安装包打进接收端，让配对网页可以直接扫码下载。
//
// scripts/build-windows.ps1 先构建 Android，再从该次 Gradle 输出复制 APK 和
// 版本清单。Verify 在接收端启动前检查完整性，缺少 APK 的构建不能启动接收。
package apkdist

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"sync"
)

//go:embed assets
var assets embed.FS

var (
	once     sync.Once
	name     string
	data     []byte
	sum      string
	metadata Metadata
)

type Metadata struct {
	VersionName       string `json:"version_name"`
	VersionCode       int    `json:"version_code"`
	SHA256            string `json:"sha256"`
	BuildType         string `json:"build_type"`
	Debuggable        *bool  `json:"debuggable"`
	CertificateSHA256 string `json:"certificate_sha256"`
}

func load() {
	once.Do(func() {
		name, data, sum = scan(assets)
		if b, err := assets.ReadFile("assets/apk.json"); err == nil {
			_ = json.Unmarshal(b, &metadata)
		}
	})
}

func Version() string           { load(); return metadata.VersionName }
func VersionCode() int          { load(); return metadata.VersionCode }
func BuildType() string         { load(); return metadata.BuildType }
func Debuggable() bool          { load(); return metadata.Debuggable == nil || *metadata.Debuggable }
func CertificateSHA256() string { load(); return metadata.CertificateSHA256 }
func Verify() error {
	load()
	return verify(data, metadata)
}

func verify(body []byte, meta Metadata) error {
	h := sha256.Sum256(body)
	if len(body) == 0 || meta.VersionName == "" || meta.VersionCode <= 0 || meta.SHA256 != hex.EncodeToString(h[:]) {
		return fmt.Errorf("内置 APK 或版本清单无效，请用 scripts/build-windows.ps1 重新构建")
	}
	certificate, err := hex.DecodeString(meta.CertificateSHA256)
	if meta.BuildType != "release" || meta.Debuggable == nil || *meta.Debuggable || err != nil || len(certificate) != sha256.Size ||
		meta.CertificateSHA256 != strings.ToLower(meta.CertificateSHA256) {
		return fmt.Errorf("内置 APK 缺少有效的 release 签名与不可调试验证，请用 scripts/build-windows.ps1 重新构建")
	}
	return nil
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
