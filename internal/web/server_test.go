package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"straightcast/internal/model"
)

type fake struct {
	snap  model.Snapshot
	err   error
	stops int
}

func (f *fake) Snapshot() model.Snapshot   { return f.snap }
func (f *fake) Apply(model.Settings) error { return f.err }
func (f *fake) Halt()                      { f.stops++ }

func TestHealthAndPageContainProduct(t *testing.T) {
	ctrl := &fake{snap: model.Snapshot{OK: true, Phase: model.PhaseStopped}}
	server := httptest.NewServer(Handler(ctrl))
	defer server.Close()

	health, err := http.Get(server.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("health %d", health.StatusCode)
	}

	page, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	body, err := io.ReadAll(page.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"Straightcast", "rtspt", "1フレーム", "/icon.svg"} {
		if !strings.Contains(text, want) {
			t.Fatalf("page missing %s", want)
		}
	}
	icon, err := http.Get(server.URL + "/icon.svg")
	if err != nil {
		t.Fatal(err)
	}
	defer icon.Body.Close()
	iconBody, err := io.ReadAll(icon.Body)
	if err != nil {
		t.Fatal(err)
	}
	if icon.StatusCode != http.StatusOK || !strings.Contains(icon.Header.Get("Content-Type"), "image/svg+xml") {
		t.Fatalf("icon %d %s", icon.StatusCode, icon.Header.Get("Content-Type"))
	}
	if !strings.Contains(string(iconBody), "#1c1b16") {
		t.Fatal("icon mark missing")
	}
}

func TestStartRejectsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(Handler(&fake{}))
	defer server.Close()
	response, err := http.Post(server.URL+"/api/start", "application/json", strings.NewReader("{"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", response.StatusCode)
	}
}

func TestPageAndStop(t *testing.T) {
	ctrl := &fake{snap: model.Snapshot{OK: true, Phase: model.PhaseLive, ID: "desk-abc", TCPURL: "rtspt://127.0.0.1:8554/desk-abc"}}
	server := httptest.NewServer(Handler(ctrl))
	defer server.Close()

	page, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	if page.StatusCode != 200 || !strings.Contains(page.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("status %d type %s", page.StatusCode, page.Header.Get("Content-Type"))
	}

	response, err := http.Post(server.URL+"/api/stop", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if ctrl.stops != 1 {
		t.Fatalf("stops %d", ctrl.stops)
	}
}

func TestStartError(t *testing.T) {
	ctrl := &fake{err: errSample}
	server := httptest.NewServer(Handler(ctrl))
	defer server.Close()
	response, err := http.Post(server.URL+"/api/start", "application/json", strings.NewReader(`{"id":"no"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", response.StatusCode)
	}
	if got := ReadError(response); got != errSample.Error() {
		t.Fatal(got)
	}
}

var errSample = sampleError{}

type sampleError struct{}

func (sampleError) Error() string { return "配信 ID が短すぎます" }
