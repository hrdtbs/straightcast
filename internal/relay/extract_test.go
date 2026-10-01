package relay

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExtractTarGZAndZip(t *testing.T) {
	payload := []byte("straightcast-bin")
	dir := t.TempDir()

	tarPath := filepath.Join(dir, "mediamtx.tar.gz")
	writeTarGZ(t, tarPath, "mediamtx", payload)
	if err := extractBinary(tarPath, dir, "mediamtx"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "mediamtx"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("tar payload %q", got)
	}

	zipPath := filepath.Join(dir, "mediamtx.zip")
	writeZip(t, zipPath, "mediamtx.exe", payload)
	if err := extractBinary(zipPath, dir, "mediamtx.exe"); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(filepath.Join(dir, "mediamtx.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("zip payload %q", got)
	}
}

func TestExtractMissingBinary(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "empty.tar.gz")
	writeTarGZ(t, tarPath, "LICENSE", []byte("x"))
	if err := extractBinary(tarPath, dir, "mediamtx"); err == nil {
		t.Fatal("expected missing binary error")
	}
}

func TestDownloadWritesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("archive"))
	}))
	defer server.Close()
	dest := filepath.Join(t.TempDir(), "file")
	if err := download(server.URL, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "archive" {
		t.Fatal(string(got))
	}
}

func TestDownloadRejectsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	err := download(server.URL, filepath.Join(t.TempDir(), "file"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnsureMediaMTXUsesExistingFile(t *testing.T) {
	t.Setenv("STRAIGHTCAST_MEDIAMTX", "")
	_, binaryName, err := MediaMTXAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, binaryName)
	if err := os.WriteFile(path, []byte("local"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := EnsureMediaMTX(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("got %s", got)
	}
}

func TestFindFFmpegEnv(t *testing.T) {
	t.Setenv("STRAIGHTCAST_FFMPEG", "/opt/straightcast/ffmpeg")
	got, err := FindFFmpeg(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != "/opt/straightcast/ffmpeg" {
		t.Fatal(got)
	}
}

func writeTarGZ(t *testing.T, path, name string, payload []byte) {
	t.Helper()
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(payload)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeZip(t *testing.T, path, name string, payload []byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}
