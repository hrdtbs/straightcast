package relay

import "testing"

func TestMediaMTXAssetNames(t *testing.T) {
	archive, binary, err := MediaMTXAsset("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if archive != "mediamtx_v1.21.1_linux_amd64.tar.gz" || binary != "mediamtx" {
		t.Fatalf("%s %s", archive, binary)
	}
	archive, binary, err = MediaMTXAsset("windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if archive != "mediamtx_v1.21.1_windows_amd64.zip" || binary != "mediamtx.exe" {
		t.Fatalf("%s %s", archive, binary)
	}
	if _, _, err := MediaMTXAsset("freebsd", "amd64"); err == nil {
		t.Fatal("expected error")
	}
}
