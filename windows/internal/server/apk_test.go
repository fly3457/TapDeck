package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
)

func apkBytes() []byte {
	b := make([]byte, 4096)
	copy(b, "PK\x03\x04")
	for i := 4; i < len(b); i++ {
		b[i] = byte(i)
	}
	return b
}

func TestAPKDownloadServesEmbeddedBytes(t *testing.T) {
	s := &Server{}
	data := apkBytes()
	s.SetAPK("TapDeck.apk", data, "abc123")
	rec := httptest.NewRecorder()
	s.apkFile(rec, httptest.NewRequest("GET", "http://192.168.1.11:41080/apk", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/vnd.android.package-archive" {
		t.Fatalf("content-type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, `filename="TapDeck.apk"`) {
		t.Fatalf("content-disposition = %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatalf("body mismatch: %d bytes", rec.Body.Len())
	}
}

// 只构建 Windows 端时没有内置 APK：下载入口要给出去 Release 的提示而不是 404 空白页。
func TestAPKDownloadWithoutEmbeddedAPK(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.apkFile(rec, httptest.NewRequest("GET", "http://192.168.1.11:41080/apk", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), ReleaseURL) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestAPKQRCodePointsAtDownloadURL(t *testing.T) {
	s := &Server{}
	s.SetAPK("TapDeck.apk", apkBytes(), "abc123")
	rec := httptest.NewRecorder()
	s.apkQR(rec, httptest.NewRequest("GET", "http://192.168.1.11:41080/apk/qr.png", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("status = %d type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	// 二维码内容必须就是手机可直接访问的下载地址。
	want, e := qrcode.Encode("http://192.168.1.11:41080/apk", qrcode.Medium, 512)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatal("二维码内容与下载地址不一致")
	}
}

func TestPairPageOffersDownload(t *testing.T) {
	s := &Server{}
	s.SetAPK("TapDeck.apk", apkBytes(), "0123456789abcdef0123")
	rec := httptest.NewRecorder()
	s.pairPage(rec, httptest.NewRequest("GET", "http://192.168.1.11:41080/pair", nil))
	page := rec.Body.String()
	for _, want := range []string{`src="/apk/qr.png"`, `href="/apk" download`, "TapDeck.apk", "0123456789abcdef", "http://192.168.1.11:41080/apk"} {
		if !strings.Contains(page, want) {
			t.Fatalf("配对页缺少 %q", want)
		}
	}
	if strings.Contains(page, "<!--APK-->") {
		t.Fatal("配对页占位符没有被替换")
	}
}

func TestPairPageWithoutAPK(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.pairPage(rec, httptest.NewRequest("GET", "http://192.168.1.11:41080/pair", nil))
	page := rec.Body.String()
	if !strings.Contains(page, ReleaseURL) {
		t.Fatalf("没有内置 APK 时应给出 Release 链接")
	}
	if strings.Contains(page, "/apk/qr.png") {
		t.Fatal("没有内置 APK 时不应显示下载二维码")
	}
}
