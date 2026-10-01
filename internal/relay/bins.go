// Package relay は MediaMTX と ffmpeg を起動します。
package relay

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const mediaMTXVersion = "v1.21.1"

// MediaMTXAsset は GitHub のリリースファイル名です。
func MediaMTXAsset(goos, goarch string) (archiveName, binaryName string, err error) {
	binaryName = "mediamtx"
	if goos == "windows" {
		binaryName = "mediamtx.exe"
	}
	key := goos + "/" + goarch
	suffix := map[string]string{
		"linux/amd64":   "linux_amd64.tar.gz",
		"linux/arm64":   "linux_arm64.tar.gz",
		"darwin/amd64":  "darwin_amd64.tar.gz",
		"darwin/arm64":  "darwin_arm64.tar.gz",
		"windows/amd64": "windows_amd64.zip",
	}[key]
	if suffix == "" {
		return "", "", fmt.Errorf("このOS用のMediaMTXはありません: %s", key)
	}
	return "mediamtx_" + mediaMTXVersion + "_" + suffix, binaryName, nil
}

// FindFFmpeg は PATH か bin ディレクトリの ffmpeg を返します。
func FindFFmpeg(binDir string) (string, error) {
	if env := os.Getenv("STRAIGHTCAST_FFMPEG"); env != "" {
		return env, nil
	}
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name = "ffmpeg.exe"
	}
	candidate := filepath.Join(binDir, name)
	if fileExists(candidate) {
		return candidate, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("ffmpegがありません。Windowsではhttps://www.gyan.dev/ffmpeg/builds/ のessentialsをPATHかbinに置いてください")
	}
	return path, nil
}

// EnsureMediaMTX は既存のバイナリを使うか、無ければ公式リリースを bin に置きます。
func EnsureMediaMTX(binDir string) (string, error) {
	if env := os.Getenv("STRAIGHTCAST_MEDIAMTX"); env != "" {
		return env, nil
	}
	_, binaryName, err := MediaMTXAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		if path, lookErr := exec.LookPath("mediamtx"); lookErr == nil {
			return path, nil
		}
		return "", err
	}
	dest := filepath.Join(binDir, binaryName)
	if fileExists(dest) {
		return dest, nil
	}
	if path, lookErr := exec.LookPath(binaryName); lookErr == nil {
		return path, nil
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	archiveName, _, err := MediaMTXAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	url := "https://github.com/bluenviron/mediamtx/releases/download/" + mediaMTXVersion + "/" + archiveName
	archivePath := filepath.Join(binDir, archiveName)
	if err := download(url, archivePath); err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(archivePath) }()
	if err := extractBinary(archivePath, binDir, binaryName); err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(dest, 0o755)
	}
	if !fileExists(dest) {
		return "", fmt.Errorf("MediaMTXを展開できません")
	}
	return dest, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func download(url, dest string) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("MediaMTXのダウンロードに失敗しました: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("MediaMTXのダウンロードに失敗しました (%d)", response.StatusCode)
	}
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, response.Body)
	return err
}

func extractBinary(archivePath, destDir, binaryName string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, destDir, binaryName)
	}
	return extractTarGZ(archivePath, destDir, binaryName)
}

func extractTarGZ(archivePath, destDir, binaryName string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return fmt.Errorf("アーカイブに%sがありません", binaryName)
		}
		if err != nil {
			return err
		}
		if filepath.Base(header.Name) != binaryName || header.FileInfo().IsDir() {
			continue
		}
		return writeFile(filepath.Join(destDir, binaryName), reader, 0o755)
	}
}

func extractZip(archivePath, destDir, binaryName string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if filepath.Base(file.Name) != binaryName || file.FileInfo().IsDir() {
			continue
		}
		body, err := file.Open()
		if err != nil {
			return err
		}
		err = writeFile(filepath.Join(destDir, binaryName), body, 0o755)
		_ = body.Close()
		return err
	}
	return fmt.Errorf("アーカイブに%sがありません", binaryName)
}

func writeFile(path string, reader io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, reader)
	return err
}
