// Package model は制御画面と中継が共有する状態です。
package model

// 配信の段階です。
const (
	PhaseStarting = "starting"
	PhaseLive     = "live"
	PhaseStopped  = "stopped"
	PhaseError    = "error"
)

// Settings は配信の設定です。
type Settings struct {
	ID          string `json:"id"`
	FPS         int    `json:"fps"`
	BitrateKbps int    `json:"bitrateKbps"`
	Monitor     int    `json:"monitor"`
	Encoder     string `json:"encoder"`
	Host        string `json:"host"`
	Window      string `json:"window"`
}

// Snapshot は制御画面が読む状態です。
type Snapshot struct {
	OK           bool   `json:"ok"`
	Phase        string `json:"phase"`
	ID           string `json:"id"`
	FPS          int    `json:"fps"`
	BitrateKbps  int    `json:"bitrateKbps"`
	Monitor      int    `json:"monitor"`
	Window       string `json:"window"`
	Encoder      string `json:"encoder"`
	EncoderLabel string `json:"encoderLabel"`
	Hardware     bool   `json:"hardware"`
	Source       string `json:"source"`
	SourceLabel  string `json:"sourceLabel"`
	Note         string `json:"note"`
	Error        string `json:"error,omitempty"`
	TCPURL       string `json:"tcpUrl"`
	UDPURL       string `json:"udpUrl"`
	RTSPPort     int    `json:"rtspPort"`
	Host         string `json:"host"`
}
