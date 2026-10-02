package expose

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

func TestRoutableWAN(t *testing.T) {
	if !routableWAN(net.ParseIP("203.0.113.10")) {
		t.Fatal("public")
	}
	for _, text := range []string{"10.1.1.1", "192.168.0.2", "172.16.4.1", "100.64.0.1", "127.0.0.1", "169.254.1.1"} {
		if routableWAN(net.ParseIP(text)) {
			t.Fatalf("accepted %s", text)
		}
	}
}

func TestParseRelayAddr(t *testing.T) {
	text := "You are not authenticated.\ntcp://mqrxt-52-11-109-186.run.pinggy-free.link:40339\n"
	host, port, ok := parseRelayAddr(text)
	if !ok || host != "mqrxt-52-11-109-186.run.pinggy-free.link" || port != 40339 {
		t.Fatalf("%s %d %v", host, port, ok)
	}
	if _, _, ok := parseRelayAddr("no address"); ok {
		t.Fatal("expected miss")
	}
}

func TestParseSTUN(t *testing.T) {
	raw := make([]byte, 32)
	raw[0] = 0x01
	raw[1] = 0x01
	raw[3] = 12
	raw[4] = 0x21
	raw[5] = 0x12
	raw[6] = 0xa4
	raw[7] = 0x42
	raw[20] = 0x00
	raw[21] = 0x20
	raw[23] = 8
	raw[25] = 0x01
	xorIP := []byte{203, 0, 113, 10}
	magic := []byte{0x21, 0x12, 0xa4, 0x42}
	raw[28] = xorIP[0] ^ magic[0]
	raw[29] = xorIP[1] ^ magic[1]
	raw[30] = xorIP[2] ^ magic[2]
	raw[31] = xorIP[3] ^ magic[3]
	ip, err := parseSTUN(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !ip.Equal(net.IPv4(203, 0, 113, 10)) {
		t.Fatalf("ip %s", ip)
	}
}

func TestParsePMP(t *testing.T) {
	external := []byte{0, 128, 0, 0, 0, 0, 0, 1, 203, 0, 113, 9}
	ip, err := parsePMPExternal(external)
	if err != nil || !ip.Equal(net.ParseIP("203.0.113.9")) {
		t.Fatalf("%s %v", ip, err)
	}
	mapped := []byte{0, 130, 0, 0, 0, 0, 0, 1, 0x21, 0x72, 0x21, 0x72, 0, 0, 0x1c, 0x20}
	port, life, err := parsePMPMap(mapped)
	if err != nil || port != 8562 || life != 7200 {
		t.Fatalf("port %d life %d err %v", port, life, err)
	}
}

func TestRouteParsers(t *testing.T) {
	ip, err := parseLinuxRoute("Iface Destination Gateway Flags\neth0 00000000 0101A8C0 0003\n")
	if err != nil || ip.String() != "192.168.1.1" {
		t.Fatalf("%s %v", ip, err)
	}
	ip, err = parseDarwinRoute("   gateway: 10.0.0.1\n")
	if err != nil || ip.String() != "10.0.0.1" {
		t.Fatalf("%s %v", ip, err)
	}
	ip, err = parseWindowsRoute("0.0.0.0  0.0.0.0  192.168.0.1  192.168.0.20  25\n")
	if err != nil || ip.String() != "192.168.0.1" {
		t.Fatalf("%s %v", ip, err)
	}
}

func TestRelayRoundTrip(t *testing.T) {
	if os.Getenv("STRAIGHTCAST_RELAY") == "" {
		t.Skip()
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("straightcast-ok"))
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sess, err := openRelay(ctx, port)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	ep := sess.Endpoint()
	t.Logf("relay %s:%d", ep.Host, ep.Port)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ep.Host, ep.Port), 8*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	buf := make([]byte, 32)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "straightcast-ok" {
		t.Fatalf("got %q", buf[:n])
	}
}

func TestOpenRejectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		_, _ = Open(ctx, 8554, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Open ignored cancellation")
	}
}
