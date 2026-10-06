//go:build windows

package driver

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"unsafe"
)

func TestEmbeddedOriginalMSIAndExtraction(t *testing.T) {
	if e := Verify(); e != nil {
		t.Fatal(e)
	}
	path, e := Extract(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if len(b) != 1089536 || fmt.Sprintf("%x", sha256.Sum256(b)) != SHA256 {
		t.Fatal("upstream MSI changed")
	}
	if e := VerifySignature(path); e != nil {
		t.Fatal("offline Authenticode verification", e)
	}
	if unsafe.Sizeof(shellExecuteInfo{}) != 112 {
		t.Fatal("SHELLEXECUTEINFO x64 ABI")
	}
}
