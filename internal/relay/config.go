package relay

import "fmt"

// MediaMTXConfig は RTSP だけを開く設定です。
// WebRTC、RTMP、HLS、SRT、MoQ はポートも変換も要らないので閉じます。
// rtspt は TCP、rtsp は UDP です。UDP を開けても TCP 側の遅延は増えません。
func MediaMTXConfig(rtspAddress string) string {
	return fmt.Sprintf(`# Straightcast。再エンコードせず、TCP の RTSP だけを配る。
logLevel: info
logDestinations: [stdout]

readTimeout: 10s
writeTimeout: 10s

authMethod: internal
authInternalUsers:
  - user: any
    pass: ""
    ips: []
    permissions:
      - action: publish
      - action: read
      - action: playback

rtsp: true
rtspAddress: %s
rtspTransports: [tcp, udp]
rtspEncryption: "no"

rtmp: false
hls: false
webrtc: false
srt: false
moq: false

paths:
  all_others:
    source: publisher
    overridePublisher: true
`, rtspAddress)
}
