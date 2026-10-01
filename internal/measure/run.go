// Package measure は RTSP を読み戻すまでの遅延を測る。
package measure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"straightcast/internal/capture"
	"straightcast/internal/clock"
	"straightcast/internal/platform"
	"straightcast/internal/relay"
)

const (
	width  = 640
	height = 352
)

// Result は1回の計測です。
type Result struct {
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	Samples        []int  `json:"samples"`
	Steady         []int  `json:"steady"`
	MedianMs       int    `json:"medianMs"`
	SteadyMedianMs int    `json:"steadyMedianMs"`
	Encoder        string `json:"encoder"`
	FFmpegCPU      string `json:"ffmpegCPU"`
	Path           string `json:"path"`
	Note           string `json:"note"`
}

// Run はテスト映像を libx264 で RTSP に出し、読み戻した時刻との差を出します。
func Run(ffmpegPath, mtxPath string, frames, fps, bitrate, port int) (Result, error) {
	if frames < 8 {
		frames = 8
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	log := &bytesBuffer{}
	mtx, dir, mtxWait, err := relay.StartMediaMTX(ctx, mtxPath, fmt.Sprintf(":%d", port), log)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	defer func() {
		platform.Kill(mtx)
		if mtxWait != nil {
			select {
			case <-mtxWait:
			case <-time.After(2 * time.Second):
			}
		}
	}()

	id := "latency"
	url := fmt.Sprintf("rtsp://127.0.0.1:%d/%s", port, id)
	args, err := capture.PublishArgs(capture.Options{
		Encoder:     capture.EncoderX264,
		Source:      capture.SourceRaw,
		FPS:         fps,
		BitrateKbps: bitrate,
		Width:       width,
		Height:      height,
		Threads:     2,
		RTSPURL:     url,
	})
	if err != nil {
		return Result{}, err
	}
	ff := exec.Command(ffmpegPath, args...)
	platform.Setup(ff)
	stdin, err := ff.StdinPipe()
	if err != nil {
		return Result{}, err
	}
	ff.Stdout = log
	ff.Stderr = log
	if err := ff.Start(); err != nil {
		return Result{}, err
	}
	platform.Deprioritize(ff)
	ffWait := make(chan error, 1)
	go func() { ffWait <- ff.Wait() }()
	defer func() {
		platform.Kill(ff)
		select {
		case <-ffWait:
		case <-time.After(2 * time.Second):
		}
	}()

	writeCtx, stopWrite := context.WithCancel(ctx)
	defer stopWrite()
	go writeClock(writeCtx, stdin, fps)

	if !waitProbe(ctx, ffmpegPath, url) {
		return Result{}, fmt.Errorf("RTSPに映像が出ません。%s", shorten(log.String()))
	}

	probedW, probedH, err := probeSize(ctx, ffmpegPath, url)
	if err != nil {
		return Result{}, err
	}
	if probedW != width || probedH != height {
		return Result{}, fmt.Errorf("映像サイズは%dx%dです。時計は%dx%dです", probedW, probedH, width, height)
	}

	samples, err := readSamples(ctx, ffmpegPath, url, frames)
	if err != nil {
		return Result{}, fmt.Errorf("%w。%s", err, shorten(log.String()))
	}
	if len(samples) < 5 {
		return Result{}, fmt.Errorf("有効なサンプルは%d枚です", len(samples))
	}
	steady := samples
	if len(samples) > 6 {
		steady = append([]int(nil), samples[3:]...)
	}
	cpu := processCPU(ff.Process.Pid)
	return Result{
		Width:          width,
		Height:         height,
		Samples:        samples,
		Steady:         steady,
		MedianMs:       median(samples),
		SteadyMedianMs: median(steady),
		Encoder:        "libx264",
		FFmpegCPU:      cpu,
		Path:           "生フレーム → libx264 zerolatency（スレッド2）→ ローカル RTSP/TCP → デコード",
		Note:           "描画から、デコーダ1スレッドでRTSPを読むまでの時間です。30fpsでは約1フレーム、60fpsでは約半分になります。プレイヤーのバッファは含みません。",
	}, nil
}

func writeClock(ctx context.Context, stdin io.WriteCloser, fps int) {
	defer stdin.Close()
	frame := make([]byte, width*height*3)
	interval := time.Second / time.Duration(fps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			clock.Paint(frame, width, height, time.Now().UnixMilli())
			if _, err := stdin.Write(frame); err != nil {
				return
			}
		}
	}
}

func waitProbe(ctx context.Context, ffmpegPath, url string) bool {
	deadline := time.Now().Add(8 * time.Second)
	ffprobe := sibling(ffmpegPath, "ffprobe")
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		probeCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		cmd := exec.CommandContext(probeCtx, ffprobe,
			"-v", "error",
			"-rtsp_transport", "tcp",
			"-select_streams", "v:0",
			"-show_entries", "stream=width",
			"-of", "csv=p=0",
			url,
		)
		out, err := cmd.Output()
		cancel()
		if err == nil {
			width, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
			if convErr == nil && width > 0 {
				return true
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func probeSize(ctx context.Context, ffmpegPath, url string) (int, int, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, sibling(ffmpegPath, "ffprobe"),
		"-v", "error",
		"-rtsp_transport", "tcp",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0",
		url,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("映像サイズを読めません: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("映像サイズを読めません: %s", strings.TrimSpace(string(out)))
	}
	w, errW := strconv.Atoi(parts[0])
	h, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil {
		return 0, 0, fmt.Errorf("映像サイズを読めません: %s", strings.TrimSpace(string(out)))
	}
	return w, h, nil
}

func readSamples(ctx context.Context, ffmpegPath, url string, frames int) ([]int, error) {
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(readCtx, ffmpegPath,
		"-hide_banner",
		"-loglevel", "error",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-probesize", "32",
		"-analyzeduration", "0",
		"-max_delay", "0",
		"-reorder_queue_size", "0",
		"-rtsp_transport", "tcp",
		"-i", url,
		"-threads", "1",
		"-an",
		"-fps_mode", "passthrough",
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"pipe:1",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytesBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	frameBytes := width * height * 3
	buf := make([]byte, 0, frameBytes*2)
	tmp := make([]byte, 64*1024)
	samples := make([]int, 0, frames)
	for len(samples) < frames {
		n, readErr := stdout.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		for len(buf) >= frameBytes && len(samples) < frames {
			stamp := clock.ReadStamp(buf[:frameBytes], width)
			age := int(time.Now().UnixMilli() - stamp)
			if age >= 0 && age < 10000 {
				samples = append(samples, age)
			}
			buf = buf[frameBytes:]
		}
		if readErr != nil {
			break
		}
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("フレームを読めません。%s", shorten(stderr.String()))
	}
	return samples, nil
}

func sibling(ffmpegPath, name string) string {
	candidate := strings.TrimSuffix(ffmpegPath, "ffmpeg") + name
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return name
}

func median(values []int) int {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]int(nil), values...)
	sort.Ints(ordered)
	return ordered[len(ordered)/2]
}

func processCPU(pid int) string {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "%cpu=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func shorten(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 500 {
		return text
	}
	return text[len(text)-500:]
}

type bytesBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.b.Len() > 20000 {
		return len(p), nil
	}
	return b.b.Write(p)
}

func (b *bytesBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// Print は結果を JSON で出します。
func Print(result Result) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}
