// Package web は配信の開始と URL のコピーだけを行う画面です。
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"straightcast/internal/model"
)

//go:embed page.html
var files embed.FS

// Controller は中継の操作です。
type Controller interface {
	Snapshot() model.Snapshot
	Apply(model.Settings) error
	Halt()
}

// Handler は制御画面と API です。
func Handler(ctrl Controller) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		page, err := files.ReadFile("page.html")
		if err != nil {
			http.Error(w, "画面を読めません", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(page)
	})
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, ctrl.Snapshot())
	})
	mux.HandleFunc("POST /api/start", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "リクエストを読めませんでした"})
			return
		}
		var settings model.Settings
		if err := json.Unmarshal(body, &settings); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "設定の形式が不正です"})
			return
		}
		if err := ctrl.Apply(settings); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ctrl.Snapshot())
	})
	mux.HandleFunc("POST /api/stop", func(w http.ResponseWriter, _ *http.Request) {
		ctrl.Halt()
		writeJSON(w, http.StatusOK, ctrl.Snapshot())
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ReadError はテストからエラー文を取ります。
func ReadError(response *http.Response) string {
	var payload struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(response.Body).Decode(&payload)
	if payload.Error == "" {
		return errors.New("empty").Error()
	}
	return payload.Error
}
