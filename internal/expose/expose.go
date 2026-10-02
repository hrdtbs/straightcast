// Package expose は RTSP を別のネットワークから届くアドレスに出します。
package expose

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
)

// 外向けの出し方です。
const (
	ModeDirect = "direct"
	ModeRelay  = "relay"
)

// Endpoint は受け側が開くアドレスです。
type Endpoint struct {
	Host string
	Port int
	Mode string
}

// Session は転送または中継を1本持っています。
type Session struct {
	mu      sync.Mutex
	ep      Endpoint
	updates chan struct{}
	cancel  context.CancelFunc
	closeFn func()
	once    sync.Once
}

// Endpoint は現在の受け側アドレスです。
func (s *Session) Endpoint() Endpoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ep
}

// Updates はアドレスが変わったときに通知します。
func (s *Session) Updates() <-chan struct{} {
	return s.updates
}

// Close は転送と中継を閉じます。
func (s *Session) Close() {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Lock()
		fn := s.closeFn
		s.closeFn = nil
		s.mu.Unlock()
		if fn != nil {
			fn()
		}
	})
}

func (s *Session) notify() {
	select {
	case s.updates <- struct{}{}:
	default:
	}
}

// Allow は受信を通す処理です。直接転送のときだけ呼びます。
type Allow func(context.Context) error

// Open はルーター転送を試し、届かないときは外向きの中継を開きます。
func Open(ctx context.Context, localPort int, allow Allow) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if localPort < 1 || localPort > 65535 {
		return nil, fmt.Errorf("ポートが不正です")
	}
	sess, err := openDirect(ctx, localPort, allow)
	if err == nil {
		return sess, nil
	}
	log.Printf("ルーター転送: %v", err)
	relay, rerr := openRelay(ctx, localPort)
	if rerr == nil {
		return relay, nil
	}
	log.Printf("中継: %v", rerr)
	return nil, fmt.Errorf("外向けの待受を開けませんでした")
}

func localIPv4() net.IP {
	conn, err := net.Dial("udp", "192.0.2.1:9")
	if err != nil {
		return nil
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil
	}
	ip := addr.IP.To4()
	if ip == nil || !routableLAN(ip) {
		return nil
	}
	return ip
}

func routableLAN(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil || v4.IsLoopback() || v4.IsUnspecified() || v4.IsMulticast() {
		return false
	}
	return true
}

func routableWAN(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil || v4.IsPrivate() || v4.IsLoopback() || v4.IsLinkLocalUnicast() || v4.IsUnspecified() || v4.IsMulticast() {
		return false
	}
	if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return true
}
