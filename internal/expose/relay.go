package expose

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	relayHost        = "a.pinggy.io:443"
	relayUser        = "tcp"
	relayFingerprint = "SHA256:nFd5rfJMGuZXvfeRzJ/BtT3TfksAxTWMajcrHRcI7AM"
)

var relayAddr = regexp.MustCompile(`(?i)tcp://([A-Za-z0-9.-]+):([0-9]{1,5})`)

func openRelay(ctx context.Context, localPort int) (*Session, error) {
	runCtx, cancel := context.WithCancel(ctx)
	sess := &Session{
		updates: make(chan struct{}, 1),
		cancel:  cancel,
	}
	ready := make(chan error, 1)
	go sess.relayLoop(runCtx, localPort, ready)
	select {
	case err := <-ready:
		if err != nil {
			sess.Close()
			return nil, err
		}
		return sess, nil
	case <-ctx.Done():
		sess.Close()
		return nil, ctx.Err()
	}
}

func (s *Session) relayLoop(ctx context.Context, localPort int, ready chan error) {
	reported := false
	report := func(err error) {
		if reported {
			return
		}
		reported = true
		select {
		case ready <- err:
		case <-ctx.Done():
		}
	}
	failures := 0
	for {
		if err := ctx.Err(); err != nil {
			report(err)
			return
		}
		ep, stop, died, err := dialRelay(ctx, localPort)
		if err != nil {
			failures++
			if !reported && failures >= 2 {
				report(err)
				return
			}
			timer := time.NewTimer(1200 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				report(ctx.Err())
				return
			case <-timer.C:
			}
			continue
		}
		failures = 0
		s.mu.Lock()
		previous := s.closeFn
		s.ep = ep
		s.closeFn = stop
		s.mu.Unlock()
		if previous != nil {
			previous()
		}
		if !reported {
			report(nil)
		} else {
			s.notify()
		}
		select {
		case <-ctx.Done():
			stop()
			return
		case <-died:
			stop()
		}
	}
}

func dialRelay(ctx context.Context, localPort int) (Endpoint, func(), <-chan struct{}, error) {
	tlsDialer := tls.Dialer{Config: &tls.Config{ServerName: "a.pinggy.io", MinVersion: tls.VersionTLS12}}
	raw, err := tlsDialer.DialContext(ctx, "tcp", relayHost)
	if err != nil {
		return Endpoint{}, nil, nil, err
	}
	config := &ssh.ClientConfig{
		User: relayUser,
		// 識別子が OpenSSH でないと、中継側が認証を通しません。
		ClientVersion:   "SSH-2.0-OpenSSH_9.6",
		HostKeyCallback: pinRelayKey,
		Timeout:         12 * time.Second,
		Auth: []ssh.AuthMethod{
			ssh.Password("0000"),
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = "0000"
				}
				return answers, nil
			}),
		},
	}
	conn, chans, reqs, err := ssh.NewClientConn(raw, relayHost, config)
	if err != nil {
		_ = raw.Close()
		return Endpoint{}, nil, nil, err
	}
	client := ssh.NewClient(conn, chans, reqs)
	listener, err := client.Listen("tcp", "localhost:0")
	if err != nil {
		_ = client.Close()
		return Endpoint{}, nil, nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		_ = listener.Close()
		_ = client.Close()
		return Endpoint{}, nil, nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		_ = listener.Close()
		_ = client.Close()
		return Endpoint{}, nil, nil, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = session.Close()
		_ = listener.Close()
		_ = client.Close()
		return Endpoint{}, nil, nil, err
	}
	lines := make(chan string, 16)
	var pumps sync.WaitGroup
	pumps.Add(2)
	go func() { defer pumps.Done(); scanLines(stdout, lines) }()
	go func() { defer pumps.Done(); scanLines(stderr, lines) }()
	go func() { pumps.Wait(); close(lines) }()
	if err := session.Shell(); err != nil {
		_ = session.Close()
		_ = listener.Close()
		_ = client.Close()
		return Endpoint{}, nil, nil, err
	}
	host, port, err := waitRelayAddr(ctx, lines)
	if err != nil {
		_ = session.Close()
		_ = listener.Close()
		_ = client.Close()
		return Endpoint{}, nil, nil, err
	}
	died := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = session.Close()
			_ = listener.Close()
			_ = client.Close()
		})
	}
	go func() {
		serveLocal(listener, localPort)
		close(died)
	}()
	go func() {
		_ = session.Wait()
		stop()
	}()
	go keepRelay(ctx, client, stop)
	return Endpoint{Host: host, Port: port, Mode: ModeRelay}, stop, died, nil
}

func pinRelayKey(_ string, _ net.Addr, key ssh.PublicKey) error {
	if ssh.FingerprintSHA256(key) != relayFingerprint {
		return fmt.Errorf("中継サーバーの鍵が一致しません")
	}
	return nil
}

func scanLines(r io.Reader, out chan<- string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		out <- scanner.Text()
	}
}

func waitRelayAddr(ctx context.Context, lines <-chan string) (string, int, error) {
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	var text string
	for {
		select {
		case <-ctx.Done():
			return "", 0, ctx.Err()
		case <-timer.C:
			return "", 0, fmt.Errorf("中継のアドレスが返りませんでした")
		case line, ok := <-lines:
			if !ok {
				return "", 0, fmt.Errorf("中継のアドレスが返りませんでした")
			}
			text += line + "\n"
			host, port, found := parseRelayAddr(text)
			if found {
				return host, port, nil
			}
		}
	}
}

func parseRelayAddr(text string) (string, int, bool) {
	match := relayAddr.FindStringSubmatch(text)
	if match == nil {
		return "", 0, false
	}
	port, err := strconv.Atoi(match[2])
	if err != nil || port < 1 || port > 65535 {
		return "", 0, false
	}
	return match[1], port, true
}

func serveLocal(ln net.Listener, localPort int) {
	target := fmt.Sprintf("127.0.0.1:%d", localPort)
	for {
		remote, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			local, err := net.DialTimeout("tcp", target, 5*time.Second)
			if err != nil {
				_ = remote.Close()
				return
			}
			splice(remote, local)
		}()
	}
}

func splice(left, right net.Conn) {
	defer left.Close()
	defer right.Close()
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(left, right)
		_ = left.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(right, left)
		_ = right.Close()
		done <- struct{}{}
	}()
	<-done
}

func keepRelay(ctx context.Context, client *ssh.Client, stop func()) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stop()
			return
		case <-ticker.C:
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				stop()
				return
			}
		}
	}
}
