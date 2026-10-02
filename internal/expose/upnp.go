package expose

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/huin/goupnp/dcps/internetgateway2"
)

const (
	mapLease   = 7200
	mapRefresh = 30 * time.Minute
)

type mapper interface {
	externalIP(context.Context) (net.IP, error)
	add(context.Context, uint16, uint16, string, uint32) error
	remove(context.Context, uint16) error
}

type ip1 struct {
	c *internetgateway2.WANIPConnection1
}
type ip2 struct {
	c *internetgateway2.WANIPConnection2
}
type ppp struct {
	c *internetgateway2.WANPPPConnection1
}

func (m ip1) externalIP(ctx context.Context) (net.IP, error) {
	return parseExternal(m.c.GetExternalIPAddressCtx(ctx))
}
func (m ip1) add(ctx context.Context, external, internal uint16, client string, lease uint32) error {
	return m.c.AddPortMappingCtx(ctx, "", external, "TCP", internal, client, true, "Straightcast", lease)
}
func (m ip1) remove(ctx context.Context, external uint16) error {
	return m.c.DeletePortMappingCtx(ctx, "", external, "TCP")
}

func (m ip2) externalIP(ctx context.Context) (net.IP, error) {
	return parseExternal(m.c.GetExternalIPAddressCtx(ctx))
}
func (m ip2) add(ctx context.Context, external, internal uint16, client string, lease uint32) error {
	return m.c.AddPortMappingCtx(ctx, "", external, "TCP", internal, client, true, "Straightcast", lease)
}
func (m ip2) remove(ctx context.Context, external uint16) error {
	return m.c.DeletePortMappingCtx(ctx, "", external, "TCP")
}

func (m ppp) externalIP(ctx context.Context) (net.IP, error) {
	return parseExternal(m.c.GetExternalIPAddressCtx(ctx))
}
func (m ppp) add(ctx context.Context, external, internal uint16, client string, lease uint32) error {
	return m.c.AddPortMappingCtx(ctx, "", external, "TCP", internal, client, true, "Straightcast", lease)
}
func (m ppp) remove(ctx context.Context, external uint16) error {
	return m.c.DeletePortMappingCtx(ctx, "", external, "TCP")
}

func parseExternal(text string, err error) (net.IP, error) {
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(text).To4()
	if ip == nil {
		return nil, fmt.Errorf("外向きアドレスが不正です")
	}
	return ip, nil
}

func discoverMappers(ctx context.Context) []mapper {
	found := make(chan []mapper, 3)
	go func() {
		search, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		clients, _, err := internetgateway2.NewWANIPConnection1ClientsCtx(search)
		out := make([]mapper, 0, len(clients))
		if err == nil {
			for _, client := range clients {
				out = append(out, ip1{client})
			}
		}
		found <- out
	}()
	go func() {
		search, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		clients, _, err := internetgateway2.NewWANIPConnection2ClientsCtx(search)
		out := make([]mapper, 0, len(clients))
		if err == nil {
			for _, client := range clients {
				out = append(out, ip2{client})
			}
		}
		found <- out
	}()
	go func() {
		search, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		clients, _, err := internetgateway2.NewWANPPPConnection1ClientsCtx(search)
		out := make([]mapper, 0, len(clients))
		if err == nil {
			for _, client := range clients {
				out = append(out, ppp{client})
			}
		}
		found <- out
	}()
	var out []mapper
	for i := 0; i < 3; i++ {
		select {
		case <-ctx.Done():
			return out
		case part := <-found:
			out = append(out, part...)
		}
	}
	return out
}

func openDirect(ctx context.Context, localPort int, allow Allow) (*Session, error) {
	clientIP := localIPv4()
	if clientIP == nil {
		return nil, fmt.Errorf("このPCのアドレスが分かりません")
	}
	mappers := discoverMappers(ctx)
	if nat, err := natpmpMapper(ctx); err == nil {
		mappers = append(mappers, nat)
	}
	if len(mappers) == 0 {
		return nil, fmt.Errorf("ルーターが見つかりません")
	}
	stunCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	stunIP, stunErr := stunIPv4(stunCtx)
	cancel()

	var last error
	for _, device := range mappers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		look, lookCancel := context.WithTimeout(ctx, 2*time.Second)
		external, err := device.externalIP(look)
		lookCancel()
		if err != nil {
			last = err
			continue
		}
		if !routableWAN(external) {
			last = fmt.Errorf("ルーターの外側が公開アドレスではありません")
			continue
		}
		if stunErr == nil && !external.Equal(stunIP) {
			last = fmt.Errorf("ルーターの先に別の変換があります")
			continue
		}
		port, err := addMapping(ctx, device, localPort, clientIP.String())
		if err != nil {
			last = err
			continue
		}
		if allow != nil {
			if err := allow(ctx); err != nil {
				drop := context.WithoutCancel(ctx)
				dropCtx, dropCancel := context.WithTimeout(drop, 2*time.Second)
				_ = device.remove(dropCtx, uint16(port))
				dropCancel()
				last = err
				continue
			}
		}
		runCtx, runCancel := context.WithCancel(ctx)
		sess := &Session{
			ep:      Endpoint{Host: external.String(), Port: port, Mode: ModeDirect},
			updates: make(chan struct{}, 1),
			cancel:  runCancel,
		}
		sess.closeFn = func() {
			dropCtx, dropCancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = device.remove(dropCtx, uint16(port))
			dropCancel()
		}
		go refreshMapping(runCtx, device, localPort, port, clientIP.String())
		return sess, nil
	}
	if last == nil {
		last = fmt.Errorf("転送を追加できません")
	}
	return nil, last
}

func addMapping(ctx context.Context, device mapper, localPort int, client string) (int, error) {
	ports := []int{localPort}
	for _, extra := range []int{localPort + 1, 45000 + (localPort % 1000)} {
		if extra > 1024 && extra < 65535 && extra != localPort {
			ports = append(ports, extra)
		}
	}
	var last error
	for _, port := range ports {
		call, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := device.add(call, uint16(port), uint16(localPort), client, mapLease)
		cancel()
		if err == nil {
			return port, nil
		}
		last = err
	}
	if last == nil {
		last = fmt.Errorf("転送を追加できません")
	}
	return 0, last
}

func refreshMapping(ctx context.Context, device mapper, localPort, externalPort int, client string) {
	ticker := time.NewTicker(mapRefresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			call, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := device.add(call, uint16(externalPort), uint16(localPort), client, mapLease)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
