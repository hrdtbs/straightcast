package capture

import (
	"strings"
	"testing"
)

func TestVBVOneFrame(t *testing.T) {
	if got := VBVBits(2500, 30); got != 83333 {
		t.Fatalf("2500kbps/30fps = %d, want 83333", got)
	}
	if got := VBVBits(6000, 60); got != 100000 {
		t.Fatalf("6000kbps/60fps = %d, want 100000", got)
	}
}

func TestCandidatesWindowsPrefersHardware(t *testing.T) {
	listed := map[Encoder]bool{
		EncoderNVENC: true,
		EncoderQSV:   true,
		EncoderAMF:   true,
		EncoderX264:  true,
	}
	got := Candidates("windows", "auto", listed)
	want := []Encoder{EncoderNVENC, EncoderQSV, EncoderAMF, EncoderX264}
	if strings.Join(encoders(got), ",") != strings.Join(encoders(want), ",") {
		t.Fatalf("got %v", got)
	}
}

func TestCandidatesLinuxIgnoresListedNVENC(t *testing.T) {
	listed := map[Encoder]bool{EncoderNVENC: true, EncoderX264: true}
	got := Candidates("linux", "auto", listed)
	if len(got) != 1 || got[0] != EncoderX264 {
		t.Fatalf("got %v", got)
	}
}

func TestNVENCArgsStayOnGPU(t *testing.T) {
	args, err := PublishArgs(Options{
		Encoder:     EncoderNVENC,
		NVENCPreset: "p1",
		Source:      SourceDesktop,
		FPS:         30,
		BitrateKbps: 2500,
		Monitor:     1,
		RTSPURL:     "rtsp://127.0.0.1:8554/desk",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(args, " ")
	for _, want := range []string{
		"ddagrab=output_idx=1:framerate=30:draw_mouse=1",
		"-c:v h264_nvenc",
		"-preset p1",
		"-tune ull",
		"-rc cbr",
		"-bf 0",
		"-g 30",
		"-delay 0",
		"-zerolatency 1",
		"-rc-lookahead 0",
		"-forced-idr 1",
		"-bufsize 83333",
		"-use_wallclock_as_timestamps 1",
		"-an",
		"-fps_mode passthrough",
		"-rtsp_transport tcp",
		"dump_extra=freq=keyframe",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	for _, banned := range []string{"hwdownload", "scale=", "gdigrab", "-c:a", "libx264"} {
		if strings.Contains(text, banned) {
			t.Fatalf("hardware path should not contain %s: %s", banned, text)
		}
	}
}

func TestLegacyNVENCPreset(t *testing.T) {
	args, err := PublishArgs(Options{
		Encoder:     EncoderNVENC,
		NVENCPreset: "llhp",
		Source:      SourceDesktop,
		FPS:         60,
		BitrateKbps: 6000,
		RTSPURL:     "rtsp://127.0.0.1:8554/desk",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(args, " ")
	if !strings.Contains(text, "-preset llhp") || !strings.Contains(text, "-g 60") {
		t.Fatal(text)
	}
}

func TestQSVAndAMFLowDelay(t *testing.T) {
	qsv, err := PublishArgs(Options{
		Encoder: EncoderQSV, Source: SourceDesktop, FPS: 30, BitrateKbps: 2500,
		RTSPURL: "rtsp://127.0.0.1:8554/a",
	})
	if err != nil {
		t.Fatal(err)
	}
	q := strings.Join(qsv, " ")
	for _, want := range []string{"-async_depth 1", "-look_ahead 0", "-low_delay_brc 1", "-scenario remotegaming", "-bf 0"} {
		if !strings.Contains(q, want) {
			t.Fatalf("qsv missing %s: %s", want, q)
		}
	}
	amf, err := PublishArgs(Options{
		Encoder: EncoderAMF, Source: SourceDesktop, FPS: 30, BitrateKbps: 2500,
		RTSPURL: "rtsp://127.0.0.1:8554/a",
	})
	if err != nil {
		t.Fatal(err)
	}
	a := strings.Join(amf, " ")
	if !strings.Contains(a, "-usage ultralowlatency") || !strings.Contains(a, "-quality speed") {
		t.Fatal(a)
	}
}

func TestSoftwareDesktopCapsCPU(t *testing.T) {
	args, err := PublishArgs(Options{
		Encoder: EncoderX264, Source: SourceDesktop, FPS: 30, BitrateKbps: 2500, Threads: 2,
		RTSPURL: "rtsp://127.0.0.1:8554/a",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(args, " ")
	for _, want := range []string{
		"hwdownload,format=bgra,scale='min(1280,iw)':-2:flags=fast_bilinear",
		"-preset ultrafast",
		"-tune zerolatency",
		"-threads 2",
		"sliced-threads=1",
		"sync-lookahead=0",
		"rc-lookahead=0",
		"repeat-headers=1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
}

func TestRawMeasureHasNoDesktopGrab(t *testing.T) {
	args, err := PublishArgs(Options{
		Encoder: EncoderX264, Source: SourceRaw, FPS: 30, BitrateKbps: 2500,
		Width: 640, Height: 352, Threads: 2,
		RTSPURL: "rtsp://127.0.0.1:8554/latency",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(args, " ")
	if strings.Contains(text, "ddagrab") || strings.Contains(text, "testsrc") || strings.Contains(text, "-re ") {
		t.Fatal(text)
	}
	if !strings.Contains(text, "pipe:0") || !strings.Contains(text, "640x352") {
		t.Fatal(text)
	}
}

func TestNormalizeRejectsBadID(t *testing.T) {
	if _, err := Normalize("A", 30, 2500, 0, "auto"); err == nil {
		t.Fatal("expected error")
	}
	got, err := Normalize("desk-1", 30, 2500, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Encoder != EncoderAuto || got.Threads != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestParseEncoderList(t *testing.T) {
	text := " V....D libx264              libx264 H.264\n V....D h264_nvenc           NVIDIA NVENC\n"
	found := ParseEncoderList(text)
	if !found[EncoderX264] || !found[EncoderNVENC] || found[EncoderAMF] {
		t.Fatal(found)
	}
}

func encoders(list []Encoder) []string {
	out := make([]string, len(list))
	for i, enc := range list {
		out[i] = string(enc)
	}
	return out
}
