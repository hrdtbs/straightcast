// Package capture は ffmpeg の引数を作ります。
package capture

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

// Encoder は H.264 の実装です。
type Encoder string

const (
	// EncoderAuto は NVENC、QSV、AMF、libx264 の順です。
	EncoderAuto Encoder = "auto"
	// EncoderNVENC は NVIDIA です。
	EncoderNVENC Encoder = "nvenc"
	// EncoderQSV は Intel Quick Sync です。
	EncoderQSV Encoder = "qsv"
	// EncoderAMF は AMD です。
	EncoderAMF Encoder = "amf"
	// EncoderX264 は libx264 です。
	EncoderX264 Encoder = "libx264"
)

// SourceKind は映像の出どころです。
type SourceKind string

const (
	// SourceDesktop は Windows の ddagrab です。
	SourceDesktop SourceKind = "desktop"
	// SourceWindow は Windows の gfxcapture です。
	SourceWindow SourceKind = "window"
	// SourceTest はテスト映像です。
	SourceTest SourceKind = "test"
	// SourceRaw は計測用の raw 入力です。
	SourceRaw SourceKind = "raw"
)

// Options は1本の配信コマンドです。
type Options struct {
	Encoder     Encoder
	NVENCPreset string // p1 または llhp です。空なら p1 です。
	Source      SourceKind
	ID          string
	FPS         int
	BitrateKbps int
	Monitor     int
	Window      string
	Width       int
	Height      int
	RTSPURL     string
	Threads     int
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)

// ValidID は RTSP のパスに使える識別子です。
func ValidID(id string) bool {
	return idPattern.MatchString(id)
}

// DefaultSource は OS で使える一番軽い取り込みです。
// Windows では DXGI Desktop Duplication を使います。それ以外はテスト映像です。
func DefaultSource(goos string) SourceKind {
	if goos == "windows" {
		return SourceDesktop
	}
	return SourceTest
}

// Hardware は NVENC、QSV、AMF なら true です。
func Hardware(enc Encoder) bool {
	switch enc {
	case EncoderNVENC, EncoderQSV, EncoderAMF:
		return true
	default:
		return false
	}
}

// EncoderLabel は画面に出す名前です。
func EncoderLabel(enc Encoder, preset string) string {
	switch enc {
	case EncoderNVENC:
		if preset == "llhp" {
			return "NVIDIA NVENC（llhp）"
		}
		return "NVIDIA NVENC"
	case EncoderQSV:
		return "Intel Quick Sync"
	case EncoderAMF:
		return "AMD AMF"
	case EncoderX264:
		return "libx264"
	default:
		return string(enc)
	}
}

// SourceLabel は取り込みの名前です。
func SourceLabel(kind SourceKind) string {
	switch kind {
	case SourceDesktop:
		return "デスクトップ"
	case SourceWindow:
		return "ウィンドウ"
	case SourceRaw:
		return "計測"
	default:
		return "テスト映像"
	}
}

// VBVBits は 1 フレーム分の VBV です。大きいとエンコーダが溜めます。
func VBVBits(kbps, fps int) int {
	if fps < 1 {
		fps = 30
	}
	if kbps < 1 {
		kbps = 2500
	}
	bits := kbps * 1000 / fps
	if bits < 8000 {
		bits = 8000
	}
	return bits
}

// Candidates は試す順です。Linux の auto では NVENC を試しません。明示したエンコーダだけ返します。
func Candidates(goos, preference string, listed map[Encoder]bool) []Encoder {
	pref := Encoder(preference)
	if pref != "" && pref != EncoderAuto {
		if listed[pref] {
			return []Encoder{pref}
		}
		return nil
	}
	if goos == "windows" {
		order := []Encoder{EncoderNVENC, EncoderQSV, EncoderAMF, EncoderX264}
		var out []Encoder
		for _, enc := range order {
			if listed[enc] {
				out = append(out, enc)
			}
		}
		return out
	}
	if listed[EncoderX264] {
		return []Encoder{EncoderX264}
	}
	return nil
}

// NVENCAttempts は p1 のあと llhp です。
func NVENCAttempts(enc Encoder) []string {
	if enc != EncoderNVENC {
		return []string{""}
	}
	return []string{"p1", "llhp"}
}

// Normalize は範囲外の設定を配信できる値に直します。
// window が空ならモニター全体です。入っていればそのタイトルを含むウィンドウです。
func Normalize(id string, fps, bitrate, monitor int, encoder, window string) (Options, error) {
	if !ValidID(id) {
		return Options{}, fmt.Errorf("配信IDが不正です")
	}
	switch fps {
	case 15, 24, 30, 60:
	default:
		return Options{}, fmt.Errorf("フレームレートが不正です")
	}
	if bitrate < 800 || bitrate > 12000 {
		return Options{}, fmt.Errorf("ビットレートが不正です")
	}
	if monitor < 0 || monitor > 8 {
		return Options{}, fmt.Errorf("モニター番号が不正です")
	}
	enc := Encoder(encoder)
	switch enc {
	case "", EncoderAuto, EncoderNVENC, EncoderQSV, EncoderAMF, EncoderX264:
		if enc == "" {
			enc = EncoderAuto
		}
	default:
		return Options{}, fmt.Errorf("不明なエンコーダです")
	}
	title, source, err := cleanWindow(window, runtime.GOOS)
	if err != nil {
		return Options{}, err
	}
	return Options{
		Encoder:     enc,
		Source:      source,
		ID:          id,
		FPS:         fps,
		BitrateKbps: bitrate,
		Monitor:     monitor,
		Window:      title,
		Width:       1280,
		Height:      720,
		Threads:     2,
	}, nil
}

func cleanWindow(window, goos string) (string, SourceKind, error) {
	window = strings.TrimSpace(window)
	if window == "" {
		return "", DefaultSource(goos), nil
	}
	if goos != "windows" {
		return "", "", fmt.Errorf("ウィンドウの取り込みはWindowsだけです")
	}
	if len([]rune(window)) > 200 {
		return "", "", fmt.Errorf("ウィンドウ名が長すぎます")
	}
	for _, r := range window {
		if r < 0x20 || r == 0x7f {
			return "", "", fmt.Errorf("ウィンドウ名に使えない文字があります")
		}
	}
	return window, SourceWindow, nil
}

// PublishArgs は ffmpeg の引数です。音声は付けません。
func PublishArgs(o Options) ([]string, error) {
	if o.RTSPURL == "" {
		return nil, fmt.Errorf("RTSPの宛先が空です")
	}
	if o.FPS < 1 {
		return nil, fmt.Errorf("フレームレートが不正です")
	}
	if o.BitrateKbps < 1 {
		return nil, fmt.Errorf("ビットレートが不正です")
	}
	if o.Threads < 1 {
		o.Threads = 2
	}
	if o.Width < 16 {
		o.Width = 1280
	}
	if o.Height < 16 {
		o.Height = 720
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-nostats",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-thread_queue_size", "1",
		"-probesize", "32",
		"-analyzeduration", "0",
	}

	// 壁時計をタイムスタンプにします。フレーム番号だと mux が次の時刻まで抱えます。
	args = append(args, "-use_wallclock_as_timestamps", "1")
	switch o.Source {
	case SourceDesktop:
		// ddagrab を使います。gdigrab は使いません。
		spec := fmt.Sprintf("ddagrab=output_idx=%d:framerate=%d:draw_mouse=1", o.Monitor, o.FPS)
		args = append(args, "-f", "lavfi", "-i", spec)
	case SourceWindow:
		// gfxcapture は d3d11 のまま出します。キャンバスサイズと fps フィルタは付けません。
		spec, err := gfxcaptureSpec(o.Window, o.FPS)
		if err != nil {
			return nil, err
		}
		args = append(args, "-f", "lavfi", "-i", spec)
	case SourceTest:
		spec := fmt.Sprintf("testsrc2=size=%dx%d:rate=%d", o.Width, o.Height, o.FPS)
		args = append(args, "-re", "-f", "lavfi", "-i", spec)
	case SourceRaw:
		args = append(args,
			"-f", "rawvideo",
			"-pix_fmt", "rgb24",
			"-video_size", fmt.Sprintf("%dx%d", o.Width, o.Height),
			"-framerate", fmt.Sprint(o.FPS),
			"-i", "pipe:0",
		)
	default:
		return nil, fmt.Errorf("不明な入力です")
	}

	args = append(args, "-an")
	args = append(args, encoderArgs(o)...)
	args = append(args,
		"-fps_mode", "passthrough",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-max_delay", "0",
		"-max_interleave_delta", "0",
		"-flush_packets", "1",
		"-bsf:v", "dump_extra=freq=keyframe",
		"-f", "rtsp",
		"-rtsp_transport", "tcp",
		o.RTSPURL,
	)
	return args, nil
}

func encoderArgs(o Options) []string {
	rate := fmt.Sprintf("%dk", o.BitrateKbps)
	buf := fmt.Sprint(VBVBits(o.BitrateKbps, o.FPS))
	gop := fmt.Sprint(o.FPS)

	switch o.Encoder {
	case EncoderNVENC:
		preset := o.NVENCPreset
		if preset == "" {
			preset = "p1"
		}
		return []string{
			"-c:v", "h264_nvenc",
			"-preset", preset,
			"-tune", "ull",
			"-rc", "cbr",
			"-b:v", rate,
			"-maxrate", rate,
			"-bufsize", buf,
			"-profile:v", "main",
			"-bf", "0",
			"-g", gop,
			"-delay", "0",
			"-zerolatency", "1",
			"-rc-lookahead", "0",
			"-no-scenecut", "1",
			"-forced-idr", "1",
			"-spatial-aq", "0",
			"-temporal-aq", "0",
		}
	case EncoderQSV:
		return []string{
			"-c:v", "h264_qsv",
			"-preset", "veryfast",
			"-async_depth", "1",
			"-look_ahead", "0",
			"-look_ahead_depth", "0",
			"-low_delay_brc", "1",
			"-scenario", "remotegaming",
			"-bf", "0",
			"-forced_idr", "1",
			"-g", gop,
			"-b:v", rate,
			"-maxrate", rate,
			"-bufsize", buf,
			"-profile:v", "main",
		}
	case EncoderAMF:
		return []string{
			"-c:v", "h264_amf",
			"-usage", "ultralowlatency",
			"-quality", "speed",
			"-rc", "cbr",
			"-bf", "0",
			"-g", gop,
			"-b:v", rate,
			"-maxrate", rate,
			"-bufsize", buf,
			"-profile:v", "main",
		}
	default:
		args := []string{}
		if o.Source == SourceDesktop || o.Source == SourceWindow {
			// libx264 のときだけ CPU に下ろし、幅を 1280 までにします。
			args = append(args, "-vf", "hwdownload,format=bgra,scale='min(1280,iw)':-2:flags=fast_bilinear")
		}
		args = append(args,
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-tune", "zerolatency",
			"-profile:v", "main",
			"-pix_fmt", "yuv420p",
			"-bf", "0",
			"-g", gop,
			"-keyint_min", gop,
			"-sc_threshold", "0",
			"-b:v", rate,
			"-maxrate", rate,
			"-bufsize", buf,
			"-threads", fmt.Sprint(o.Threads),
			"-x264-params", "nal-hrd=cbr:repeat-headers=1:sliced-threads=1:sync-lookahead=0:rc-lookahead=0",
		)
		return args
	}
}

// gfxcaptureSpec はウィンドウタイトルの一部一致です。大文字小文字は区別しません。
func gfxcaptureSpec(window string, fps int) (string, error) {
	window = strings.TrimSpace(window)
	if window == "" {
		return "", fmt.Errorf("ウィンドウ名が空です")
	}
	if fps < 1 {
		return "", fmt.Errorf("フレームレートが不正です")
	}
	// 引用の中で正規表現の特殊文字を文字として扱います。幅と高さは渡しません。
	pattern := lavfiQuote("(?i).*" + regexLiteral(window) + ".*")
	return fmt.Sprintf(
		"gfxcapture=window_title=%s:capture_cursor=1:capture_border=1:max_framerate=%d",
		pattern, fps,
	), nil
}

func regexLiteral(text string) string {
	var b strings.Builder
	for _, r := range text {
		if strings.ContainsRune(`\^$.|?*+()[]{}`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func lavfiQuote(text string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range text {
		if r == '\\' || r == '\'' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
	return b.String()
}

// HasFilter は `ffmpeg -filters` の出力に名前があるかを見ます。
func HasFilter(text, name string) bool {
	for _, line := range strings.Split(text, "\n") {
		for _, field := range strings.Fields(line) {
			if field == name {
				return true
			}
		}
	}
	return false
}

// ParseEncoderList は `ffmpeg -encoders` の出力から使える実装を拾います。
func ParseEncoderList(text string) map[Encoder]bool {
	found := map[Encoder]bool{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[1]
		switch name {
		case "h264_nvenc":
			found[EncoderNVENC] = true
		case "h264_qsv":
			found[EncoderQSV] = true
		case "h264_amf":
			found[EncoderAMF] = true
		case "libx264":
			found[EncoderX264] = true
		}
	}
	return found
}
