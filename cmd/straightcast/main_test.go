package main

import (
	"os"
	"path/filepath"
	"testing"

	"straightcast/internal/capture"
)

func TestRandomIDIsUsable(t *testing.T) {
	for i := 0; i < 20; i++ {
		id := randomID()
		if !capture.ValidID(id) {
			t.Fatalf("id %s", id)
		}
	}
}

func TestExistingBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mediamtx")
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := existingBinary([]string{dir}, []string{"mediamtx"}); got != path {
		t.Fatalf("got %s", got)
	}
	if got := existingBinary([]string{dir}, []string{"missing"}); got != "" {
		t.Fatalf("got %s", got)
	}
}

func TestBinDirsNotEmpty(t *testing.T) {
	if len(binDirs()) == 0 {
		t.Fatal("empty")
	}
}
