// Package capture は ffmpeg の取り込みと低遅延エンコードの引数を組み立てます。
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
	// EncoderAuto は使えるハードウェアを順に試し、無ければ libx264 です。
	EncoderAuto Encoder = "auto"
	// EncoderNVENC は NVIDIA の固定機能エンコーダです。
	EncoderNVENC Encoder = "nvenc"
	// EncoderQSV は Intel Quick Sync です。
	EncoderQSV Encoder = "qsv"
	// EncoderAMF は AMD の固定機能エンコーダです。
	EncoderAMF Encoder = "amf"
	// EncoderX264 はソフトウェアエンコードです。
	EncoderX264 Encoder = "libx264"
)

// SourceKind は映像の出どころです。
type SourceKind string

const (
	// SourceDesktop は Windows の DXGI Desktop Duplication です。
	SourceDesktop SourceKind = "desktop"
	// SourceTest はデスクトップを取れない環境のテスト映像です。
	SourceTest SourceKind = "test"
	// SourceRaw は計測用に標準入力から受け取る映像です。
	SourceRaw SourceKind = "raw"
)

// Options は1本の配信コマンドです。
type Options struct {
	Encoder     Encoder
	NVENCPreset string // p1 または llhp。空なら p1。
	Source      SourceKind
	ID          string
	FPS         int
	BitrateKbps int
	Monitor     int
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
// Windows では DXGI Desktop Duplication。それ以外はテスト映像です。
func DefaultSource(goos string) SourceKind {
	if goos == "windows" {
		return SourceDesktop
	}
	return SourceTest
}

// Hardware は専用のエンコード回路を使う実装です。
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
			return "NVIDIA NVENC（低遅延・旧プリセット）"
		}
		return "NVIDIA NVENC（超低遅延）"
	case EncoderQSV:
		return "Intel Quick Sync"
	case EncoderAMF:
		return "AMD AMF（超低遅延）"
	case EncoderX264:
		return "ソフトウェア（libx264・スレッド 2）"
	default:
		return string(enc)
	}
}

// SourceLabel は画面に出す取り込みの説明です。
func SourceLabel(kind SourceKind) string {
	switch kind {
	case SourceDesktop:
		return "この PC のデスクトップ（DXGI）"
	case SourceRaw:
		return "計測用の映像"
	default:
		return "テスト映像"
	}
}

// VBVBits は1フレーム分の VBV バッファです。
// これより大きいと、エンコーダが先のフレームを溜めてから出します。
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

// Candidates は試す順のエンコーダです。
// Linux では一覧に NVENC があっても使いません。ドライバが無いことが多いのと、
// デスクトップ取り込みが Windows 限定なためです。明示指定のときはその1つだけです。
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

// NVENCAttempts は新しいプリセットが無い ffmpeg 向けに、失敗したら旧プリセットを試します。
func NVENCAttempts(enc Encoder) []string {
	if enc != EncoderNVENC {
		return []string{""}
	}
	return []string{"p1", "llhp"}
}

// Normalize は範囲外の設定を配信できる値に直します。
func Normalize(id string, fps, bitrate, monitor int, encoder string) (Options, error) {
	if !ValidID(id) {
		return Options{}, fmt.Errorf("配信 ID は英小文字・数字・ハイフンで 3〜63 文字にしてください")
	}
	switch fps {
	case 15, 24, 30, 60:
	default:
		return Options{}, fmt.Errorf("フレームレートは 15、24、30、60 のどれかにしてください")
	}
	if bitrate < 800 || bitrate > 12000 {
		return Options{}, fmt.Errorf("ビットレートは 800〜12000 kbps にしてください")
	}
	if monitor < 0 || monitor > 8 {
		return Options{}, fmt.Errorf("モニター番号は 0 から 8 にしてください")
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
	return Options{
		Encoder:     enc,
		Source:      DefaultSource(runtime.GOOS),
		ID:          id,
		FPS:         fps,
		BitrateKbps: bitrate,
		Monitor:     monitor,
		Width:       1280,
		Height:      720,
		Threads:     2,
	}, nil
}

// PublishArgs は ffmpeg に渡す引数です。音声は付けません。
func PublishArgs(o Options) ([]string, error) {
	if o.RTSPURL == "" {
		return nil, fmt.Errorf("RTSP の宛先が空です")
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

	// 入力のタイムスタンプを「届いた時刻」にする。
	// フレーム番号で刻むと、マクスが次の時刻までフレームを抱える。
	args = append(args, "-use_wallclock_as_timestamps", "1")
	switch o.Source {
	case SourceDesktop:
		// gdigrab は CPU で画面をコピーする。ddagrab は DXGI のフレームをそのまま渡す。
		spec := fmt.Sprintf("ddagrab=output_idx=%d:framerate=%d:draw_mouse=1", o.Monitor, o.FPS)
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
		if o.Source == SourceDesktop {
			// D3D11 のフレームをソフトウェアエンコーダへ渡すときだけ、CPU 側へ下ろす。
			// 幅は 1280 までに抑え、CPU を取りすぎないようにする。
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
