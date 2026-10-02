package expose

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type pmp struct {
	gateway  net.IP
	internal uint16
}

func (p *pmp) externalIP(ctx context.Context) (net.IP, error) {
	buf, err := p.exchange(ctx, []byte{0, 0})
	if err != nil {
		return nil, err
	}
	return parsePMPExternal(buf)
}

func (p *pmp) add(ctx context.Context, external, internal uint16, _ string, lease uint32) error {
	p.internal = internal
	req := make([]byte, 12)
	req[1] = 2
	binary.BigEndian.PutUint16(req[4:6], internal)
	binary.BigEndian.PutUint16(req[6:8], external)
	binary.BigEndian.PutUint32(req[8:12], lease)
	buf, err := p.exchange(ctx, req)
	if err != nil {
		return err
	}
	_, _, err = parsePMPMap(buf)
	return err
}

func (p *pmp) remove(ctx context.Context, external uint16) error {
	req := make([]byte, 12)
	req[1] = 2
	binary.BigEndian.PutUint16(req[4:6], p.internal)
	binary.BigEndian.PutUint16(req[6:8], external)
	buf, err := p.exchange(ctx, req)
	if err != nil {
		return err
	}
	if len(buf) < 4 || buf[0] != 0 || buf[1] != 130 {
		return fmt.Errorf("転送応答が不正です")
	}
	if code := binary.BigEndian.Uint16(buf[2:4]); code != 0 {
		return fmt.Errorf("転送応答が失敗しました")
	}
	return nil
}

func (p pmp) exchange(ctx context.Context, req []byte) ([]byte, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", net.JoinHostPort(p.gateway.String(), "5351"))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(800 * time.Millisecond)
	}
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func natpmpMapper(ctx context.Context) (mapper, error) {
	gateway, err := defaultGateway(ctx)
	if err != nil {
		return nil, err
	}
	return &pmp{gateway: gateway}, nil
}

func parsePMPExternal(buf []byte) (net.IP, error) {
	if len(buf) < 12 || buf[0] != 0 || buf[1] != 128 {
		return nil, fmt.Errorf("アドレス応答が不正です")
	}
	if code := binary.BigEndian.Uint16(buf[2:4]); code != 0 {
		return nil, fmt.Errorf("アドレス応答が失敗しました")
	}
	ip := net.IPv4(buf[8], buf[9], buf[10], buf[11]).To4()
	return ip, nil
}

func parsePMPMap(buf []byte) (int, uint32, error) {
	if len(buf) < 16 || buf[0] != 0 || buf[1] != 130 {
		return 0, 0, fmt.Errorf("転送応答が不正です")
	}
	if code := binary.BigEndian.Uint16(buf[2:4]); code != 0 {
		return 0, 0, fmt.Errorf("転送応答が失敗しました")
	}
	port := int(binary.BigEndian.Uint16(buf[10:12]))
	life := binary.BigEndian.Uint32(buf[12:16])
	if port < 1 {
		return 0, 0, fmt.Errorf("転送ポートが不正です")
	}
	return port, life, nil
}

func defaultGateway(ctx context.Context) (net.IP, error) {
	switch runtime.GOOS {
	case "linux":
		text, err := os.ReadFile("/proc/net/route")
		if err != nil {
			return nil, err
		}
		return parseLinuxRoute(string(text))
	case "darwin":
		out, err := exec.CommandContext(ctx, "route", "-n", "get", "default").Output()
		if err != nil {
			return nil, err
		}
		return parseDarwinRoute(string(out))
	case "windows":
		out, err := exec.CommandContext(ctx, "route", "print", "-4").Output()
		if err != nil {
			return nil, err
		}
		return parseWindowsRoute(string(out))
	default:
		return nil, fmt.Errorf("経路が分かりません")
	}
}

func parseLinuxRoute(text string) (net.IP, error) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[1] != "00000000" {
			continue
		}
		value, err := strconv.ParseUint(fields[2], 16, 32)
		if err != nil {
			continue
		}
		ip := net.IPv4(byte(value), byte(value>>8), byte(value>>16), byte(value>>24)).To4()
		if ip != nil && !ip.IsUnspecified() {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("経路が分かりません")
}

func parseDarwinRoute(text string) (net.IP, error) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "gateway:") {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))).To4()
		if ip != nil {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("経路が分かりません")
}

func parseWindowsRoute(text string) (net.IP, error) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "0.0.0.0" || fields[1] != "0.0.0.0" {
			continue
		}
		ip := net.ParseIP(fields[2]).To4()
		if ip != nil && !ip.IsUnspecified() {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("経路が分かりません")
}
