package server

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"net"
	"testing"

	"tapdeck/internal/secure"
)

func TestVirtualAdapters(t *testing.T) {
	mac := func(s string) net.HardwareAddr { m, _ := net.ParseMAC(s); return m }
	for _, c := range []struct {
		name    string
		mac     string
		virtual bool
	}{
		{"WLAN", "a4:42:3b:01:02:03", false},
		{"以太网", "a4:42:3b:01:02:04", false},
		{"vEthernet (WSL (Hyper-V firewall))", "00:15:5d:aa:bb:cc", true},
		{"以太网 3", "00:15:5d:aa:bb:cd", true}, // Hyper-V 虚拟交换机改过名
		{"VMware Network Adapter VMnet8", "00:50:56:c0:00:08", true},
		{"VirtualBox Host-Only Network", "0a:00:27:00:00:11", true},
		{"Meta", "", true}, // Clash Meta TUN
		{"Tailscale", "", true},
	} {
		if got := isVirtualAdapter(c.name, mac(c.mac)); got != c.virtual {
			t.Errorf("%s %s: virtual=%v，期望 %v", c.name, c.mac, got, c.virtual)
		}
	}
}

func TestRankAddressesPrefersRealLAN(t *testing.T) {
	ip := net.ParseIP
	cands := []addressCandidate{
		{ip: ip("192.168.160.1").To4(), name: "vEthernet (WSL)", virtual: true},
		{ip: ip("198.18.0.1").To4(), name: "Meta", virtual: true},
		{ip: ip("100.101.102.103").To4(), name: "以太网 9"},
		{ip: ip("192.168.56.1").To4(), name: "以太网 2"},
		{ip: ip("192.168.1.11").To4(), name: "WLAN"},
	}
	got := rankAddresses(cands, ip("192.168.1.11"))
	want := []string{"192.168.1.11", "192.168.56.1", "100.101.102.103", "192.168.160.1", "198.18.0.1"}
	for i := range want {
		if got[i].String() != want[i] {
			t.Fatalf("排序 %v，期望 %v", got, want)
		}
	}
	// 没有默认路由时按字符串排序，仍然先真实网卡、再虚拟网卡。
	got = rankAddresses(cands[:2:2], nil)
	if got[0].String() != "192.168.160.1" {
		t.Fatalf("只有虚拟网卡时也应给出地址：%v", got)
	}
}

func TestPrivateLAN(t *testing.T) {
	for s, want := range map[string]bool{"10.0.0.5": true, "172.16.1.1": true, "172.31.255.1": true, "172.32.0.1": false, "192.168.1.11": true, "198.18.0.1": false, "100.64.0.1": false} {
		if privateLAN(net.ParseIP(s)) != want {
			t.Errorf("%s 应为 %v", s, want)
		}
	}
}

// 电脑换网络后证书补上新地址，私钥与指纹不变，原有地址保留。
func TestCertificateAddsNewAddressesKeepingPin(t *testing.T) {
	dir := t.TempDir()
	old := net.ParseIP("203.0.113.7")
	cert, pin, err := secure.Certificate(dir, []net.IP{old})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{dir: dir, cert: cert, pin: pin}
	got, err := s.certificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(got.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	has := func(ip net.IP) bool {
		for _, v := range leaf.IPAddresses {
			if v.Equal(ip) {
				return true
			}
		}
		return false
	}
	for _, ip := range append(LocalIPs(), old) {
		if !has(ip) {
			t.Fatalf("证书缺少 %s：%v", ip, leaf.IPAddresses)
		}
	}
	spki, _ := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if sum := sha256.Sum256(spki); hex.EncodeToString(sum[:]) != pin {
		t.Fatal("重新签发后指纹变化")
	}
	again, _ := s.certificate(nil)
	if &again.Certificate[0][0] != &got.Certificate[0][0] {
		t.Fatal("地址未变化时不应重新签发")
	}
}
