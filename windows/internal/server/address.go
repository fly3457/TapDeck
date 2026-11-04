package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"sort"
	"strings"

	"tapdeck/internal/secure"
)

// addressCandidate 是一块网卡上的一个 IPv4 地址。
type addressCandidate struct {
	ip      net.IP
	name    string
	mac     net.HardwareAddr
	virtual bool
}

// 虚拟网卡（WSL、Hyper-V、虚拟机、容器、VPN 隧道）的名称关键字与 MAC 前缀：手机通常访问不到这些地址。
var (
	virtualNames = []string{"vethernet", "wsl", "hyper-v", "vmware", "virtualbox", "vbox", "docker", "loopback", "tailscale", "zerotier", "wireguard", "vpn", "tap-", "tun", "clash", "mihomo", "meta"}
	virtualMACs  = [][]byte{{0x00, 0x15, 0x5d}, {0x00, 0x50, 0x56}, {0x00, 0x0c, 0x29}, {0x00, 0x05, 0x69}, {0x08, 0x00, 0x27}, {0x0a, 0x00, 0x27}, {0x02, 0x42}}
)

func isVirtualAdapter(name string, mac net.HardwareAddr) bool {
	n := strings.ToLower(name)
	for _, k := range virtualNames {
		if strings.Contains(n, k) {
			return true
		}
	}
	for _, p := range virtualMACs {
		if bytes.HasPrefix(mac, p) {
			return true
		}
	}
	return false
}

// privateLAN 判断是否为家庭／办公局域网常用的私有地址；198.18.0.0/15（代理 TUN）与 100.64.0.0/10（CGNAT／Tailscale）不算。
func privateLAN(ip net.IP) bool {
	v := ip.To4()
	return v != nil && (v[0] == 10 || v[0] == 172 && v[1]&0xf0 == 16 || v[0] == 192 && v[1] == 168)
}

// rankAddresses 按手机最可能访问到的顺序排列：真实网卡优先，其次私有局域网段，再次默认路由所在地址，最后按字符串排序保证稳定。
func rankAddresses(cands []addressCandidate, route net.IP) []net.IP {
	cands = append([]addressCandidate(nil), cands...)
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.virtual != b.virtual {
			return !a.virtual
		}
		if pa, pb := privateLAN(a.ip), privateLAN(b.ip); pa != pb {
			return pa
		}
		if ra, rb := a.ip.Equal(route), b.ip.Equal(route); ra != rb {
			return ra
		}
		return a.ip.String() < b.ip.String()
	})
	out := make([]net.IP, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.ip)
	}
	return out
}

// routeIP 返回系统默认路由使用的本机地址；UDP Dial 只查路由表，不发送数据。
func routeIP() net.IP {
	c, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return nil
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
		return a.IP.To4()
	}
	return nil
}

// LocalIPs 返回本机 IPv4 地址：第一个固定为 127.0.0.1，其后按 [rankAddresses] 排序，第二个即配对网址使用的地址。
func LocalIPs() []net.IP {
	var cands []addressCandidate
	is, _ := net.Interfaces()
	for _, i := range is {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		as, _ := i.Addrs()
		for _, a := range as {
			ip, _, e := net.ParseCIDR(a.String())
			if e == nil && ip.To4() != nil && !ip.IsLinkLocalUnicast() {
				cands = append(cands, addressCandidate{ip: ip.To4(), name: i.Name, mac: i.HardwareAddr, virtual: isVirtualAdapter(i.Name, i.HardwareAddr)})
			}
		}
	}
	return append([]net.IP{net.ParseIP("127.0.0.1").To4()}, rankAddresses(cands, routeIP())...)
}

// currentHost 是配对网址与 pair-info 中的地址：每次实时计算，电脑换网络或 Wi-Fi 晚于接收端连上时自动更新。
// s.host 非空时（测试）固定使用它。需持有 s.mu。
func (s *Server) currentHostLocked() string {
	if s.host != "" {
		return s.host
	}
	if ips := LocalIPs(); len(ips) > 1 {
		return ips[1].String()
	}
	return "127.0.0.1"
}

// certificate 供 TLS 握手使用：本机出现证书里没有的新地址时，用同一私钥重新签发（指纹不变，已配对手机不受影响），
// 地址只增不减，旧网络的地址仍然有效。
func (s *Server) certificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.certMu.Lock()
	defer s.certMu.Unlock()
	leaf, err := x509.ParseCertificate(s.cert.Certificate[0])
	if err != nil {
		return &s.cert, nil
	}
	ips := append([]net.IP(nil), leaf.IPAddresses...)
	missing := false
	for _, ip := range LocalIPs() {
		known := false
		for _, have := range ips {
			if have.Equal(ip) {
				known = true
				break
			}
		}
		if !known {
			ips = append(ips, ip)
			missing = true
		}
	}
	if !missing {
		return &s.cert, nil
	}
	cert, pin, err := secure.Certificate(s.dir, ips)
	if err != nil || pin != s.pin {
		log.Printf("证书地址更新失败：%v", err)
		return &s.cert, nil
	}
	s.cert = cert
	return &s.cert, nil
}
