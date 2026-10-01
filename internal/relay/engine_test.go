package relay

import (
	"strings"
	"testing"

	"straightcast/internal/capture"
	"straightcast/internal/model"
)

func TestPortOf(t *testing.T) {
	port, err := portOf(":8554")
	if err != nil || port != 8554 {
		t.Fatalf("port %d err %v", port, err)
	}
	port, err = portOf("127.0.0.1:8554")
	if err != nil || port != 8554 {
		t.Fatalf("port %d err %v", port, err)
	}
	if _, err := portOf("bad"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := portOf(":99999"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCleanHost(t *testing.T) {
	host, err := cleanHost("  ")
	if err != nil || host != "127.0.0.1" {
		t.Fatalf("%s %v", host, err)
	}
	host, err = cleanHost("192.168.1.20")
	if err != nil || host != "192.168.1.20" {
		t.Fatalf("%s %v", host, err)
	}
	if _, err := cleanHost("http://example"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewRejectsBadBind(t *testing.T) {
	if _, err := New("ffmpeg", "mediamtx", "nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestApplyRejectsBadSettings(t *testing.T) {
	engine, err := New("ffmpeg", "mediamtx", ":8554")
	if err != nil {
		t.Fatal(err)
	}
	err = engine.Apply(model.Settings{ID: "Bad", FPS: 30, BitrateKbps: 2500, Encoder: "auto"})
	if err == nil {
		t.Fatal("expected error")
	}
	if engine.Snapshot().Phase != model.PhaseStopped {
		t.Fatalf("phase %s", engine.Snapshot().Phase)
	}
}

func TestLiveNoteMentionsDesktopOnWindowsPath(t *testing.T) {
	note := liveNote(capture.SourceDesktop, true)
	if !strings.Contains(note, "GPU") {
		t.Fatal(note)
	}
	if liveNote(capture.SourceTest, false) == note {
		t.Fatal("test source should differ")
	}
}

func TestTailKeepsTheEnd(t *testing.T) {
	log := newTail()
	_, _ = log.Write([]byte(strings.Repeat("a", 10000)))
	_, _ = log.Write([]byte(strings.Repeat("b", 3000)))
	if !strings.HasSuffix(log.String(), strings.Repeat("b", 3000)) {
		t.Fatal("tail dropped the newest bytes")
	}
	if len(log.String()) > 12000 {
		t.Fatalf("len %d", len(log.String()))
	}
	log.Reset()
	if log.String() != "" {
		t.Fatal("reset left data")
	}
}
