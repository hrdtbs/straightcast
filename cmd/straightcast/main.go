package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"straightcast/internal/measure"
	"straightcast/internal/model"
	"straightcast/internal/relay"
	"straightcast/internal/web"
)

// version はリリース時に -ldflags で上書きします。
var version = "dev"

func main() {
	log.SetFlags(0)
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "-version" || os.Args[1] == "--version") {
		fmt.Println(version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "measure" {
		if err := runMeasure(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := serve(); err != nil {
		log.Fatal(err)
	}
}

func serve() error {
	httpAddr := flag.String("http", "127.0.0.1:43123", "制御画面の待受")
	rtspBind := flag.String("rtsp", ":8554", "RTSP の待受")
	id := flag.String("id", "", "配信 ID。空なら自動")
	fps := flag.Int("fps", 30, "フレームレート（15, 24, 30, 60）")
	bitrate := flag.Int("bitrate", 2500, "ビットレート kbps")
	monitor := flag.Int("monitor", 0, "モニター番号")
	window := flag.String("window", "", "ウィンドウタイトルの一部。空ならモニター全体")
	encoder := flag.String("encoder", "auto", "auto, nvenc, qsv, amf, libx264")
	noStart := flag.Bool("no-start", false, "起動時に配信しない")
	flag.Parse()

	ffmpegPath, mtxPath, err := findTools()
	if err != nil {
		return err
	}
	engine, err := relay.New(ffmpegPath, mtxPath, *rtspBind)
	if err != nil {
		return err
	}
	if *id == "" {
		*id = randomID()
	}

	server := &http.Server{
		Addr:              *httpAddr,
		Handler:           web.Handler(engine),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		engine.Halt()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	if !*noStart {
		settings := model.Settings{
			ID:          *id,
			FPS:         *fps,
			BitrateKbps: *bitrate,
			Monitor:     *monitor,
			Window:      *window,
			Encoder:     *encoder,
		}
		go func() {
			if err := engine.Apply(settings); err != nil {
				log.Printf("配信を開始できません: %v", err)
			} else {
				snap := engine.Snapshot()
				log.Print(snap.EncoderLabel)
				log.Print("共有URLは操作画面に出ます。")
			}
		}()
	}

	log.Printf("制御画面 http://%s", *httpAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func runMeasure(args []string) error {
	fs := flag.NewFlagSet("measure", flag.ContinueOnError)
	frames := fs.Int("frames", 18, "読むフレーム数")
	fps := fs.Int("fps", 30, "フレームレート")
	bitrate := fs.Int("bitrate", 2500, "ビットレート kbps")
	port := fs.Int("port", 8554, "RTSP ポート")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ffmpegPath, mtxPath, err := findTools()
	if err != nil {
		return err
	}
	result, err := measure.Run(ffmpegPath, mtxPath, *frames, *fps, *bitrate, *port)
	if err != nil {
		return err
	}
	return measure.Print(result)
}

func findTools() (string, string, error) {
	dirs := binDirs()
	var ffmpegPath string
	for _, dir := range dirs {
		path, err := relay.FindFFmpeg(dir)
		if err == nil {
			ffmpegPath = path
			break
		}
	}
	if ffmpegPath == "" {
		return "", "", fmt.Errorf("ffmpegがありません。Windowsではhttps://www.gyan.dev/ffmpeg/builds/ のessentialsをPATHかbinに置いてください")
	}
	if path := existingBinary(dirs, mediaMTXNames()); path != "" {
		return ffmpegPath, path, nil
	}
	mtxPath, err := relay.EnsureMediaMTX(dirs[0])
	if err != nil {
		return "", "", err
	}
	return ffmpegPath, mtxPath, nil
}

func binDirs() []string {
	var dirs []string
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(wd, "bin"))
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "bin"))
	}
	if len(dirs) == 0 {
		dirs = append(dirs, "bin")
	}
	return dirs
}

func mediaMTXNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"mediamtx.exe"}
	}
	return []string{"mediamtx"}
}

func existingBinary(dirs, names []string) string {
	for _, dir := range dirs {
		for _, name := range names {
			path := filepath.Join(dir, name)
			info, err := os.Stat(path)
			if err == nil && !info.IsDir() {
				return path
			}
		}
	}
	if path, err := exec.LookPath(names[0]); err == nil {
		return path
	}
	return ""
}

func randomID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return fmt.Sprintf("desk-%x", buf)
}
