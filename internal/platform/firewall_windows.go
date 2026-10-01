//go:build windows

package platform

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

var allowOnce sync.Once

// AllowInbound は mediamtx.exe への受信を許可します。権限が無いときは何もしません。
func AllowInbound(program string) {
	allowOnce.Do(func() {
		path, err := filepath.Abs(program)
		if err != nil || path == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		del := exec.CommandContext(ctx, "netsh", "advfirewall", "firewall", "delete", "rule", "name=Straightcast RTSP")
		Setup(del)
		_ = del.Run()
		add := exec.CommandContext(ctx, "netsh", "advfirewall", "firewall", "add", "rule",
			"name=Straightcast RTSP",
			"dir=in",
			"action=allow",
			"program="+`"`+path+`"`,
			"enable=yes",
			"profile=any",
		)
		Setup(add)
		_ = add.Run()
	})
}
