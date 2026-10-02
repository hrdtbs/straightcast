package relay

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"straightcast/internal/capture"
	"straightcast/internal/expose"
	"straightcast/internal/model"
	"straightcast/internal/platform"
)

// Engine は MediaMTX と ffmpeg を動かします。
type Engine struct {
	mu sync.Mutex
	op sync.Mutex

	ffmpegPath string
	mtxPath    string
	rtspBind   string
	rtspPort   int

	gen         int
	cancel      context.CancelFunc
	reachCancel context.CancelFunc
	reach       *expose.Session
	mtx         *exec.Cmd
	ff          *exec.Cmd
	dir         string
	log         *tail
	snap        model.Snapshot
}

// New はまだ配信していないエンジンです。
func New(ffmpegPath, mtxPath, rtspBind string) (*Engine, error) {
	port, err := portOf(rtspBind)
	if err != nil {
		return nil, err
	}
	return &Engine{
		ffmpegPath: ffmpegPath,
		mtxPath:    mtxPath,
		rtspBind:   rtspBind,
		rtspPort:   port,
		log:        newTail(),
		snap: model.Snapshot{
			OK:       true,
			Phase:    model.PhaseStopped,
			RTSPPort: port,
			Host:     SuggestedHost(),
			Encoder:  "auto",
		},
	}, nil
}

// Snapshot は制御画面向けの状態です。
func (e *Engine) Snapshot() model.Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snap
}

// Apply は設定を反映します。取り込みが同じなら URL のホストだけ替えます。
func (e *Engine) Apply(settings model.Settings) error {
	e.op.Lock()
	defer e.op.Unlock()

	host, err := cleanHost(settings.Host)
	if err != nil {
		return err
	}
	opt, err := capture.Normalize(settings.ID, settings.FPS, settings.BitrateKbps, settings.Monitor, settings.Encoder, settings.Window)
	if err != nil {
		return err
	}
	opt.RTSPURL = publishURL(e.rtspPort, opt.ID)

	e.mu.Lock()
	current := e.snap
	e.mu.Unlock()
	if current.Phase == model.PhaseLive &&
		current.ID == opt.ID &&
		current.FPS == opt.FPS &&
		current.BitrateKbps == opt.BitrateKbps &&
		current.Monitor == opt.Monitor &&
		current.Window == opt.Window &&
		current.Encoder == string(opt.Encoder) {
		e.mu.Lock()
		e.snap.Host = host
		e.snap.TCPURL = tcpURL(host, e.rtspPort, opt.ID)
		e.snap.UDPURL = udpURL(host, e.rtspPort, opt.ID)
		e.mu.Unlock()
		return nil
	}

	e.stopLocked()
	return e.startLocked(opt, host)
}

// Halt は配信を止めます。
func (e *Engine) Halt() {
	e.op.Lock()
	defer e.op.Unlock()
	e.stopLocked()
	e.mu.Lock()
	e.snap.Phase = model.PhaseStopped
	e.snap.OK = true
	e.snap.Error = ""
	e.snap.Note = "停止しています。"
	e.snap.PublicURL = ""
	e.snap.Reach = ""
	e.snap.ReachNote = ""
	e.mu.Unlock()
}

func (e *Engine) startLocked(opt capture.Options, host string) error {
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.gen++
	gen := e.gen
	e.cancel = cancel
	e.snap = model.Snapshot{
		OK:          true,
		Phase:       model.PhaseStarting,
		ID:          opt.ID,
		FPS:         opt.FPS,
		BitrateKbps: opt.BitrateKbps,
		Monitor:     opt.Monitor,
		Window:      opt.Window,
		Encoder:     string(opt.Encoder),
		Host:        host,
		RTSPPort:    e.rtspPort,
		TCPURL:      tcpURL(host, e.rtspPort, opt.ID),
		UDPURL:      udpURL(host, e.rtspPort, opt.ID),
		Source:      string(opt.Source),
		SourceLabel: capture.SourceLabel(opt.Source),
		Note:        "起動しています。",
	}
	e.mu.Unlock()

	mtx, dir, mtxWait, err := launchMediaMTX(ctx, e.mtxPath, e.rtspBind, e.log)
	if err != nil {
		cancel()
		e.fail(gen, err)
		return err
	}
	e.mu.Lock()
	if e.gen != gen {
		e.mu.Unlock()
		platform.Kill(mtx)
		_ = os.RemoveAll(dir)
		return fmt.Errorf("起動が中断されました")
	}
	e.mtx = mtx
	e.dir = dir
	e.mu.Unlock()

	used, preset, ff, wait, err := e.launchFFmpeg(ctx, opt)
	if err != nil {
		e.stopLocked()
		e.fail(e.currentGen(), err)
		return err
	}

	label := capture.EncoderLabel(used, preset)
	hardware := capture.Hardware(used)
	e.mu.Lock()
	if e.gen != gen {
		e.mu.Unlock()
		platform.Kill(ff)
		return fmt.Errorf("起動が中断されました")
	}
	e.ff = ff
	e.snap.Phase = model.PhaseLive
	e.snap.OK = true
	e.snap.Error = ""
	e.snap.Encoder = string(opt.Encoder)
	e.snap.EncoderLabel = label
	e.snap.Hardware = hardware
	e.snap.Note = liveNote(opt.Source, hardware)
	e.snap.PublicURL = ""
	e.snap.Reach = model.ReachPending
	e.snap.ReachNote = "外向けのURLを用意しています。"
	e.mu.Unlock()

	go e.watch(gen, wait, "映像の送信")
	go e.watch(gen, mtxWait, "RTSP")
	go e.serveReach(gen, opt.ID)
	return nil
}

func (e *Engine) watch(gen int, waited <-chan error, name string) {
	if waited == nil {
		return
	}
	err := <-waited
	if err == nil {
		return
	}
	e.mu.Lock()
	if e.gen != gen || e.snap.Phase == model.PhaseStopped {
		e.mu.Unlock()
		return
	}
	reach := e.reach
	reachCancel := e.reachCancel
	e.reach = nil
	e.reachCancel = nil
	e.snap.Phase = model.PhaseError
	e.snap.OK = false
	e.snap.Error = fmt.Sprintf("%sが止まりました。%s", name, shorten(e.log.String()))
	e.snap.PublicURL = ""
	e.snap.Reach = ""
	e.snap.ReachNote = ""
	e.mu.Unlock()
	if reachCancel != nil {
		reachCancel()
	}
	if reach != nil {
		reach.Close()
	}
}

func (e *Engine) fail(gen int, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.gen != gen && gen != 0 {
		return
	}
	e.snap.Phase = model.PhaseError
	e.snap.OK = false
	e.snap.Error = err.Error()
}

func (e *Engine) currentGen() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.gen
}

func (e *Engine) stopLocked() {
	e.mu.Lock()
	e.gen++
	cancel := e.cancel
	reachCancel := e.reachCancel
	reach := e.reach
	mtx := e.mtx
	ff := e.ff
	dir := e.dir
	e.cancel = nil
	e.reachCancel = nil
	e.reach = nil
	e.mtx = nil
	e.ff = nil
	e.dir = ""
	e.mu.Unlock()
	if reachCancel != nil {
		reachCancel()
	}
	if reach != nil {
		reach.Close()
	}
	if cancel != nil {
		cancel()
	}
	platform.Kill(ff)
	platform.Kill(mtx)
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
}

func (e *Engine) serveReach(gen int, id string) {
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	if e.gen != gen {
		e.mu.Unlock()
		cancel()
		return
	}
	e.reachCancel = cancel
	e.mu.Unlock()

	sess, err := expose.Open(ctx, e.rtspPort, func(ctx context.Context) error {
		return platform.AllowInbound(ctx, e.mtxPath, e.rtspPort)
	})
	if err != nil {
		log.Printf("外向けURL: %v", err)
	}
	e.mu.Lock()
	if e.gen != gen {
		e.mu.Unlock()
		cancel()
		if sess != nil {
			sess.Close()
		}
		return
	}
	if err != nil || sess == nil {
		e.snap.Reach = model.ReachFailed
		e.snap.ReachNote = "外向けのURLを用意できませんでした。同じネットワークのURLを使ってください。"
		e.snap.PublicURL = ""
		e.mu.Unlock()
		cancel()
		return
	}
	e.reach = sess
	e.applyReachLocked(id)
	e.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-sess.Updates():
			if !ok {
				return
			}
			e.mu.Lock()
			if e.gen != gen {
				e.mu.Unlock()
				return
			}
			e.applyReachLocked(id)
			e.mu.Unlock()
		}
	}
}

func (e *Engine) applyReachLocked(id string) {
	if e.reach == nil {
		return
	}
	ep := e.reach.Endpoint()
	if ep.Host == "" || ep.Port == 0 {
		return
	}
	e.snap.PublicURL = tcpURL(ep.Host, ep.Port, id)
	switch ep.Mode {
	case expose.ModeDirect:
		e.snap.Reach = model.ReachDirect
		e.snap.ReachNote = "ルーターがTCPを転送しています。別のネットワークの人に、このURLを渡してください。"
	case expose.ModeRelay:
		e.snap.Reach = model.ReachRelay
		e.snap.ReachNote = "中継を通して別のネットワークへ届きます。アドレスは約60分で変わることがあります。"
	default:
		e.snap.Reach = ep.Mode
		e.snap.ReachNote = "別のネットワークの人に、このURLを渡してください。"
	}
	log.Printf("共有 %s", e.snap.PublicURL)
}

func (e *Engine) launchFFmpeg(ctx context.Context, opt capture.Options) (capture.Encoder, string, *exec.Cmd, <-chan error, error) {
	listed := encoderList(e.ffmpegPath)
	preference := string(opt.Encoder)
	if opt.Source == capture.SourceWindow && !capture.HasFilter(filterText(e.ffmpegPath), "gfxcapture") {
		return "", "", nil, nil, fmt.Errorf("このffmpegにgfxcaptureがありません。gyan.devの新しいessentialsが必要です")
	}
	candidates := capture.Candidates(runtime.GOOS, preference, listed)
	if len(candidates) == 0 {
		return "", "", nil, nil, fmt.Errorf("H.264エンコーダがありません。ffmpegにlibx264、またはNVENC、QSV、AMFが必要です")
	}
	var last error
	for _, enc := range candidates {
		for _, preset := range capture.NVENCAttempts(enc) {
			if ctx.Err() != nil {
				return "", "", nil, nil, ctx.Err()
			}
			attempt := opt
			attempt.Encoder = enc
			attempt.NVENCPreset = preset
			args, err := capture.PublishArgs(attempt)
			if err != nil {
				last = err
				continue
			}
			e.log.Reset()
			cmd := exec.Command(e.ffmpegPath, args...)
			platform.Setup(cmd)
			cmd.Stdout = e.log
			cmd.Stderr = e.log
			if err := cmd.Start(); err != nil {
				last = err
				continue
			}
			platform.Deprioritize(cmd)
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			select {
			case err := <-exited:
				last = fmt.Errorf("%sを起動できません。%v %s", capture.EncoderLabel(enc, preset), err, shorten(e.log.String()))
				continue
			case <-ctx.Done():
				platform.Kill(cmd)
				return "", "", nil, nil, ctx.Err()
			case <-time.After(1200 * time.Millisecond):
			}
			if probeVideo(ctx, siblingTool(e.ffmpegPath, "ffprobe"), attempt.RTSPURL) {
				return enc, preset, cmd, exited, nil
			}
			platform.Kill(cmd)
			select {
			case <-exited:
			case <-time.After(2 * time.Second):
			}
			last = fmt.Errorf("%sからRTSPへ映像が出ていません。%s", capture.EncoderLabel(enc, preset), shorten(e.log.String()))
		}
	}
	if last == nil {
		last = fmt.Errorf("エンコーダを開始できません")
	}
	return "", "", nil, nil, last
}

// StartMediaMTX は RTSP だけを待つ MediaMTX を起動します。
func StartMediaMTX(ctx context.Context, mtxPath, bind string, log io.Writer) (*exec.Cmd, string, <-chan error, error) {
	return launchMediaMTX(ctx, mtxPath, bind, log)
}

func launchMediaMTX(ctx context.Context, mtxPath, bind string, log io.Writer) (*exec.Cmd, string, <-chan error, error) {
	dir, err := os.MkdirTemp("", "straightcast-")
	if err != nil {
		return nil, "", nil, err
	}
	configPath := filepath.Join(dir, "mediamtx.yml")
	if err := os.WriteFile(configPath, []byte(MediaMTXConfig(bind)), 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return nil, "", nil, err
	}
	cmd := exec.Command(mtxPath, configPath)
	cmd.Dir = dir
	platform.Setup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, "", nil, err
	}
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return nil, "", nil, fmt.Errorf("MediaMTXを起動できません: %w", err)
	}
	platform.Deprioritize(cmd)
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	ready := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		var once sync.Once
		for scanner.Scan() {
			line := scanner.Text()
			_, _ = fmt.Fprintln(log, line)
			if strings.Contains(line, "[RTSP] started") {
				once.Do(func() { close(ready) })
			}
		}
	}()
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	select {
	case <-ready:
		return cmd, dir, waited, nil
	case err := <-waited:
		_ = os.RemoveAll(dir)
		if err == nil {
			err = fmt.Errorf("すぐに終了しました")
		}
		return nil, "", nil, fmt.Errorf("MediaMTXが終了しました。%v", err)
	case <-timer.C:
		platform.Kill(cmd)
		select {
		case <-waited:
		case <-time.After(2 * time.Second):
		}
		_ = os.RemoveAll(dir)
		return nil, "", nil, fmt.Errorf("RTSPを開く前にタイムアウトしました")
	case <-ctx.Done():
		platform.Kill(cmd)
		select {
		case <-waited:
		case <-time.After(2 * time.Second):
		}
		_ = os.RemoveAll(dir)
		return nil, "", nil, ctx.Err()
	}
}

func probeVideo(ctx context.Context, ffprobe, rtspURL string) bool {
	deadline := time.Now().Add(4 * time.Second)
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
			rtspURL,
		)
		out, err := cmd.Output()
		cancel()
		if err == nil {
			width, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
			if convErr == nil && width > 0 {
				return true
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(250 * time.Millisecond):
		}
	}
	return false
}

func filterText(ffmpegPath string) string {
	cmd := exec.Command(ffmpegPath, "-hide_banner", "-filters")
	out, _ := cmd.Output()
	return string(out)
}

func encoderList(ffmpegPath string) map[capture.Encoder]bool {
	cmd := exec.Command(ffmpegPath, "-hide_banner", "-encoders")
	out, _ := cmd.Output()
	return capture.ParseEncoderList(string(out))
}

func siblingTool(ffmpegPath, name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		name += ".exe"
	}
	candidate := filepath.Join(filepath.Dir(ffmpegPath), name)
	if fileExists(candidate) {
		return candidate
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return name
}

func publishURL(port int, id string) string {
	return fmt.Sprintf("rtsp://127.0.0.1:%d/%s", port, id)
}

func tcpURL(host string, port int, id string) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("rtsp://%s:%d/%s", host, port, id)
}

func udpURL(host string, port int, id string) string {
	return fmt.Sprintf("rtsp://%s:%d/%s", host, port, id)
}

func portOf(bind string) (int, error) {
	if !strings.Contains(bind, ":") {
		bind = ":" + bind
	}
	_, portText, err := net.SplitHostPort(bind)
	if err != nil {
		return 0, fmt.Errorf("RTSPの待受アドレスが不正です: %s", bind)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("RTSPのポートが不正です")
	}
	return port, nil
}

func cleanHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return SuggestedHost(), nil
	}
	if len(host) > 253 || strings.ContainsAny(host, " /\t\r\n") || strings.Contains(host, "://") {
		return "", fmt.Errorf("ホスト名が不正です")
	}
	return host, nil
}

func liveNote(source capture.SourceKind, hardware bool) string {
	if source == capture.SourceWindow {
		if !hardware {
			return "libx264です。スレッド数は2で、幅は1280までに縮小します。"
		}
		return "指定したウィンドウをGPUで送っています。そのウィンドウを閉じると停止です。"
	}
	if source != capture.SourceDesktop {
		return "このOSではデスクトップを取得できません。テスト映像を送っています。画面の取り込みはWindowsのddagrabです。"
	}
	if !hardware {
		return "libx264です。スレッド数は2で、幅は1280までに縮小します。"
	}
	return "GPUのエンコーダを使っています。タブを閉じても配信は続きます。"
}

func shorten(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 400 {
		return text
	}
	return text[len(text)-400:]
}

type tail struct {
	mu sync.Mutex
	b  strings.Builder
}

func newTail() *tail { return &tail{} }

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b.Write(p)
	if t.b.Len() > 12000 {
		kept := t.b.String()
		t.b.Reset()
		t.b.WriteString(kept[len(kept)-6000:])
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.b.String()
}

func (t *tail) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b.Reset()
}
