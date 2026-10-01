package relay

import "net"

// SuggestedHost は他のPCから届く、このPCのIPv4です。無ければ 127.0.0.1 です。
func SuggestedHost() string {
	conn, err := net.Dial("udp", "192.0.2.1:9")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "127.0.0.1"
	}
	return hostFromIP(addr.IP)
}

func hostFromIP(ip net.IP) string {
	if ip == nil {
		return "127.0.0.1"
	}
	v4 := ip.To4()
	if v4 == nil || v4.IsLoopback() || v4.IsUnspecified() || v4.IsLinkLocalUnicast() {
		return "127.0.0.1"
	}
	return v4.String()
}
