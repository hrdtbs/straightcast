//go:build windows

package platform

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// AllowInbound は mediamtx.exe の TCP 待受を受信許可します。
func AllowInbound(ctx context.Context, program string, port int) error {
	path, err := filepath.Abs(program)
	if err != nil || path == "" || port < 1 || port > 65535 {
		return fmt.Errorf("受信許可の対象が不正です")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	name := fmt.Sprintf("Straightcast %d", port)
	if ruleMatches(ctx, name, path) {
		return nil
	}
	del := []string{"advfirewall", "firewall", "delete", "rule", "name=" + name}
	add := []string{
		"advfirewall", "firewall", "add", "rule",
		"name=" + name,
		"dir=in",
		"action=allow",
		"program=" + `"` + path + `"`,
		"protocol=TCP",
		"localport=" + strconv.Itoa(port),
		"enable=yes",
		"profile=any",
	}
	_ = runNetsh(ctx, del, false)
	if err := runNetsh(ctx, add, false); err == nil && ruleMatches(ctx, name, path) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_ = runNetsh(ctx, del, true)
	if err := runNetsh(ctx, add, true); err != nil {
		return fmt.Errorf("受信を許可できません")
	}
	if !ruleMatches(ctx, name, path) {
		return fmt.Errorf("受信を許可できません")
	}
	return nil
}

func ruleMatches(ctx context.Context, name, path string) bool {
	look, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(look, "netsh", "advfirewall", "firewall", "show", "rule", "name="+name, "verbose")
	Setup(cmd)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(path))
}

func runNetsh(ctx context.Context, args []string, elevate bool) error {
	if !elevate {
		cmd := exec.CommandContext(ctx, "netsh", args...)
		Setup(cmd)
		return cmd.Run()
	}
	var script strings.Builder
	script.WriteString("Start-Process -FilePath netsh -WindowStyle Hidden -Verb RunAs -Wait -ArgumentList ")
	for i, arg := range args {
		if i > 0 {
			script.WriteByte(',')
		}
		script.WriteByte('\'')
		script.WriteString(strings.ReplaceAll(arg, "'", "''"))
		script.WriteByte('\'')
	}
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script.String())
	Setup(cmd)
	return cmd.Run()
}
