package server

import (
	"bytes"
	"fmt"
	"html"
	"net/http"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// ReleaseURL 是接收端没有内置 APK 时给出的下载入口。
const ReleaseURL = "https://github.com/fly3457/TapDeck/releases"

// SetAPK 注入内置的 Android 安装包（内容、建议文件名与 SHA-256）。
// 未注入时配对网页只显示 Release 链接。
func (s *Server) SetAPK(name string, data []byte, sum string, version ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apkName, s.apkData, s.apkSHA = name, data, sum
	if len(version) > 0 {
		s.apkVersion = version[0]
	}
}

func (s *Server) apkInfo() (string, []byte, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.apkName, s.apkData, s.apkSHA
}

// apkFile 提供内置 APK 的下载：/apk。
func (s *Server) apkFile(w http.ResponseWriter, r *http.Request) {
	name, data, _ := s.apkInfo()
	if len(data) == 0 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "本次构建的接收端没有内置 APK。\n请到 %s 下载 Android 端安装包。\n", ReleaseURL)
		return
	}
	if name == "" {
		name = "TapDeck.apk"
	}
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Header().Set("Cache-Control", "no-store")
	// 支持断点续传，手机浏览器下载中断后可以重试。
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

// apkQR 返回下载地址的二维码：/apk/qr.png。
// 地址用请求里的 Host，因此手机从哪个地址打开配对页，二维码就指向哪个地址。
func (s *Server) apkQR(w http.ResponseWriter, r *http.Request) {
	png, e := qrcode.Encode(s.apkURL(r), qrcode.Medium, 512)
	if e != nil {
		http.Error(w, "二维码生成失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// apkURL 是手机可直接访问的 APK 下载地址。
func (s *Server) apkURL(r *http.Request) string {
	host := r.Host
	if host == "" {
		s.mu.Lock()
		host = fmt.Sprintf("%s:%d", s.host, s.cfg.HTTPPort)
		s.mu.Unlock()
	}
	return "http://" + host + "/apk"
}

// apkSection 生成配对页里的下载区块：有内置 APK 时给出二维码与直链，
// 没有时退回到 Release 链接。
func (s *Server) apkSection(r *http.Request) string {
	name, data, sum := s.apkInfo()
	if len(data) == 0 {
		return `<h2>下载 Android 端</h2><p>本次构建的接收端没有内置安装包，请到 <a class="plain" href="` + ReleaseURL + `">GitHub Releases</a> 下载 APK 后安装。</p>`
	}
	if name == "" {
		name = "TapDeck.apk"
	}
	size := fmt.Sprintf("%.1f MB", float64(len(data))/(1024*1024))
	s.mu.Lock()
	version := s.apkVersion
	s.mu.Unlock()
	return `<h2>下载 Android 端</h2>` +
		`<img class="qr" src="/apk/qr.png" alt="APK 下载二维码" width="220" height="220">` +
		`<p>首次使用请下载安装；安装完成后返回本页打开 TapDeck 连接。</p>` +
		`<a href="/apk" download>下载 Android APK · ` + html.EscapeString(version) + `（` + size + `）</a>` +
		`<p class="hint">文件：` + html.EscapeString(name) + `<br>下载地址：` + html.EscapeString(s.apkURL(r)) + `<br>SHA-256：` + html.EscapeString(sum) + `<br>安装时若提示「未知来源」，请按系统提示允许本次安装。</p>`
}
