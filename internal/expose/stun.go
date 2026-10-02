package expose

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
)

const stunMagic = 0x2112A442

func stunIPv4(ctx context.Context) (net.IP, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", "stun.l.google.com:19302")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if ok {
		_ = conn.SetDeadline(deadline)
	}
	req := make([]byte, 20)
	req[0] = 0
	req[1] = 1
	binary.BigEndian.PutUint32(req[4:8], stunMagic)
	if _, err := rand.Read(req[8:]); err != nil {
		return nil, err
	}
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return parseSTUN(buf[:n])
}

func parseSTUN(buf []byte) (net.IP, error) {
	if len(buf) < 20 {
		return nil, fmt.Errorf("応答が短すぎます")
	}
	msgType := binary.BigEndian.Uint16(buf[0:2])
	if msgType != 0x0101 {
		return nil, fmt.Errorf("応答の種類が違います")
	}
	length := int(binary.BigEndian.Uint16(buf[2:4]))
	if 20+length > len(buf) {
		return nil, fmt.Errorf("応答の長さが違います")
	}
	offset := 20
	end := 20 + length
	for offset+4 <= end {
		attr := binary.BigEndian.Uint16(buf[offset : offset+2])
		size := int(binary.BigEndian.Uint16(buf[offset+2 : offset+4]))
		offset += 4
		if offset+size > end {
			return nil, fmt.Errorf("属性が切れています")
		}
		body := buf[offset : offset+size]
		offset += size
		if size%4 != 0 {
			offset += 4 - size%4
		}
		if attr != 0x0020 && attr != 0x0001 {
			continue
		}
		ip, err := parseSTUNAddr(body, attr == 0x0020)
		if err == nil {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("外向きアドレスがありません")
}

func parseSTUNAddr(body []byte, xor bool) (net.IP, error) {
	if len(body) < 8 || body[1] != 0x01 {
		return nil, fmt.Errorf("IPv4ではありません")
	}
	raw := binary.BigEndian.Uint32(body[4:8])
	if xor {
		raw ^= stunMagic
	}
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, raw)
	return ip, nil
}
