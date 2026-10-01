package relay

import (
	"net"
	"testing"
)

func TestHostFromIP(t *testing.T) {
	if got := hostFromIP(nil); got != "127.0.0.1" {
		t.Fatal(got)
	}
	if got := hostFromIP(net.ParseIP("127.0.0.1")); got != "127.0.0.1" {
		t.Fatal(got)
	}
	if got := hostFromIP(net.ParseIP("169.254.1.1")); got != "127.0.0.1" {
		t.Fatal(got)
	}
	if got := hostFromIP(net.ParseIP("192.168.1.20")); got != "192.168.1.20" {
		t.Fatal(got)
	}
	if got := SuggestedHost(); got == "" {
		t.Fatal("empty host")
	}
}
