package relay

import (
	"strings"
	"testing"
)

func TestMediaMTXConfigDisablesExtraProtocols(t *testing.T) {
	text := MediaMTXConfig(":8554")
	for _, want := range []string{
		"rtspAddress: :8554",
		"rtspTransports: [tcp, udp]",
		"webrtc: false",
		"rtmp: false",
		"hls: false",
		"srt: false",
		"moq: false",
		"action: publish",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s", want)
		}
	}
}
