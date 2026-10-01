package capture

import (
	"runtime"
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
	if _, err := Normalize("A", 30, 2500, 0, "auto", ""); err == nil {
		t.Fatal("expected error")
	}
	got, err := Normalize("desk-1", 30, 2500, 0, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if got.Encoder != EncoderAuto || got.Threads != 2 || got.Window != "" {
		t.Fatalf("%+v", got)
	}
	if got.Source != DefaultSource(runtime.GOOS) {
		t.Fatalf("source %s", got.Source)
	}
}

func TestWindowNVENCStaysOnGPU(t *testing.T) {
	args, err := PublishArgs(Options{
		Encoder:     EncoderNVENC,
		NVENCPreset: "p1",
		Source:      SourceWindow,
		Window:      "Notepad",
		FPS:         30,
		BitrateKbps: 2500,
		RTSPURL:     "rtsp://127.0.0.1:8554/desk",
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := inputSpec(t, args)
	text := strings.Join(args, " ")
	for _, want := range []string{
		"gfxcapture=window_title='(?i).*Notepad.*'",
		"capture_cursor=1",
		"capture_border=1",
		"max_framerate=30",
		"-c:v h264_nvenc",
		"-tune ull",
		"-bufsize 83333",
		"-fps_mode passthrough",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	for _, banned := range []string{
		"hwdownload", "scale=", "gdigrab", "ddagrab", "display_border",
		"width=", "height=", ",fps", " fps=", "-vf",
	} {
		if strings.Contains(spec, banned) || strings.Contains(text, banned) {
			t.Fatalf("window hardware path should not contain %s: %s", banned, text)
		}
	}
}

func TestWindowTitleIsLiteral(t *testing.T) {
	args, err := PublishArgs(Options{
		Encoder: EncoderNVENC, Source: SourceWindow,
		Window:      `C:\App (1): it's`,
		FPS:         60,
		BitrateKbps: 2500,
		RTSPURL:     "rtsp://127.0.0.1:8554/desk",
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := inputSpec(t, args)
	want := `window_title='(?i).*C:\\\\App \\(1\\): it\'s.*':capture_cursor=1:capture_border=1:max_framerate=60`
	if !strings.Contains(spec, want) {
		t.Fatalf("spec %s", spec)
	}
	if strings.Contains(spec, "display_border") || strings.Contains(spec, ",fps") {
		t.Fatal(spec)
	}
}

func TestSoftwareWindowDownloads(t *testing.T) {
	args, err := PublishArgs(Options{
		Encoder: EncoderX264, Source: SourceWindow, Window: "Calc",
		FPS: 30, BitrateKbps: 2500, Threads: 2,
		RTSPURL: "rtsp://127.0.0.1:8554/a",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(args, " ")
	if !strings.Contains(text, "gfxcapture=") || !strings.Contains(text, "hwdownload,format=bgra") {
		t.Fatal(text)
	}
	if strings.Contains(text, "gdigrab") || strings.Contains(text, "ddagrab") {
		t.Fatal(text)
	}
}

func TestEmptyWindowSpecFails(t *testing.T) {
	_, err := PublishArgs(Options{
		Encoder: EncoderNVENC, Source: SourceWindow, FPS: 30, BitrateKbps: 2500,
		RTSPURL: "rtsp://127.0.0.1:8554/a",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeWindow(t *testing.T) {
	title, source, err := cleanWindow("  Notepad  ", "windows")
	if err != nil || title != "Notepad" || source != SourceWindow {
		t.Fatalf("%s %s %v", title, source, err)
	}
	if _, _, err := cleanWindow(strings.Repeat("あ", 201), "windows"); err == nil {
		t.Fatal("expected length error")
	}
	if _, _, err := cleanWindow("bad\nname", "windows"); err == nil {
		t.Fatal("expected char error")
	}
	if runtime.GOOS == "windows" {
		got, err := Normalize("desk-1", 30, 2500, 2, "nvenc", "Notepad")
		if err != nil || got.Source != SourceWindow || got.Monitor != 2 {
			t.Fatalf("%+v %v", got, err)
		}
		return
	}
	if _, err := Normalize("desk-1", 30, 2500, 0, "auto", "Notepad"); err == nil {
		t.Fatal("expected error")
	}
}

func TestHasFilter(t *testing.T) {
	text := " ... gfxcapture       |->V       Capture windows\n T.C scale             V->V       Scale the input video\n"
	if !HasFilter(text, "gfxcapture") || !HasFilter(text, "scale") {
		t.Fatal("missing filter")
	}
	if HasFilter(text, "ddagrab") || HasFilter("notgfxcapture", "gfxcapture") {
		t.Fatal("false match")
	}
}

func TestParseEncoderList(t *testing.T) {
	text := " V....D libx264              libx264 H.264\n V....D h264_nvenc           NVIDIA NVENC\n"
	found := ParseEncoderList(text)
	if !found[EncoderX264] || !found[EncoderNVENC] || found[EncoderAMF] {
		t.Fatal(found)
	}
}

func inputSpec(t *testing.T, args []string) string {
	t.Helper()
	for i, arg := range args {
		if arg == "-i" && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatal("no input")
	return ""
}

func encoders(list []Encoder) []string {
	out := make([]string, len(list))
	for i, enc := range list {
		out[i] = string(enc)
	}
	return out
}
