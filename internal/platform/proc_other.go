//go:build linux

// Package platform は子プロセスの優先度と終了を OS ごとに扱います。
package platform

import (
	"os/exec"
	"syscall"
	"time"
)

// Setup は子プロセスを別グループにし、親が死んだら一緒に終了させます。
func Setup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
}

// Deprioritize は nice 10。
func Deprioritize(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, 10)
}

// Kill はプロセスグループごと止めます。
func Kill(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	time.Sleep(150 * time.Millisecond)
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
