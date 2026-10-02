package relay

import "fmt"

// MediaMTXConfig は RTSP 以外を閉じた設定を返します。
func MediaMTXConfig(rtspAddress string) string {
	return fmt.Sprintf(`# Straightcast。再エンコードしません。
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
rtspTransports: [tcp]
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
