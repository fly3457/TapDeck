//go:build windows

package secure

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/windows"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"tapdeck/internal/config"
	"time"
	"unsafe"
)

var crypt32 = windows.NewLazySystemDLL("crypt32.dll")

type blob struct {
	Size uint32
	Data *byte
}

func Protected(data []byte, decrypt bool) ([]byte, error) {
	in := blob{Size: uint32(len(data)), Data: &data[0]}
	var out blob
	name := "CryptProtectData"
	if decrypt {
		name = "CryptUnprotectData"
	}
	r, _, e := crypt32.NewProc(name).Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, e
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.Data))))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}
func Certificate(dir string, ips []net.IP) (tls.Certificate, string, error) {
	path := filepath.Join(dir, "identity.dpapi")
	b, e := os.ReadFile(path)
	var key *ecdsa.PrivateKey
	if os.IsNotExist(e) {
		key, e = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			return tls.Certificate{}, "", e
		}
		raw, e := x509.MarshalECPrivateKey(key)
		if e != nil {
			return tls.Certificate{}, "", e
		}
		b, e = Protected(raw, false)
		if e != nil {
			return tls.Certificate{}, "", e
		}
		if e = os.MkdirAll(dir, 0700); e != nil {
			return tls.Certificate{}, "", e
		}
		if e = config.AtomicWrite(path, b); e != nil {
			return tls.Certificate{}, "", e
		}
	} else if e != nil {
		return tls.Certificate{}, "", e
	} else {
		raw, e := Protected(b, true)
		if e != nil {
			return tls.Certificate{}, "", e
		}
		key, e = x509.ParseECPrivateKey(raw)
		if e != nil {
			return tls.Certificate{}, "", e
		}
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	t := x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "TapDeck"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(5, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IPAddresses: ips, DNSNames: []string{"localhost"}}
	der, e := x509.CreateCertificate(rand.Reader, &t, &t, &key.PublicKey, key)
	if e != nil {
		return tls.Certificate{}, "", e
	}
	spki, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	pin := sha256.Sum256(spki)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, hex.EncodeToString(pin[:]), nil
}
func Random(n int) []byte {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(fmt.Sprintf("secure random: %v", e))
	}
	return b
}
func Hash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
