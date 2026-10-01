//go:build windows

// Package platform は子プロセスの優先度と終了を OS ごとに扱います。
package platform

import (
	"os"
	"os/exec"
	"reflect"
	"syscall"
	"unsafe"
)

const (
	createNoWindow             = 0x08000000
	belowNormalPriorityClass   = 0x00004000
	jobObjectExtendedLimitInfo = 9
	jobObjectLimitKillOnClose  = 0x2000
)

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobBasicLimit struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobExtendedLimit struct {
	Basic                 jobBasicLimit
	Io                    ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var (
	kernel32  = syscall.NewLazyDLL("kernel32.dll")
	createJob = kernel32.NewProc("CreateJobObjectW")
	setJob    = kernel32.NewProc("SetInformationJobObject")
	assignJob = kernel32.NewProc("AssignProcessToJobObject")
	job       syscall.Handle
)

// Setup はコンソールを出さず、優先度を下げる。
func Setup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | belowNormalPriorityClass,
	}
}

// Deprioritize はジョブに入れ、アプリ終了時に子プロセスも閉じます。
func Deprioritize(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if job == 0 {
		handle, _, _ := createJob.Call(0, 0)
		if handle == 0 {
			return
		}
		job = syscall.Handle(handle)
		info := jobExtendedLimit{}
		info.Basic.LimitFlags = jobObjectLimitKillOnClose
		_, _, _ = setJob.Call(uintptr(job), jobObjectExtendedLimitInfo, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	}
	process := processHandle(cmd.Process)
	if process != 0 {
		_, _, _ = assignJob.Call(uintptr(job), process)
	}
}

// Kill はプロセスを止めます。
func Kill(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

func processHandle(process *os.Process) uintptr {
	value := reflect.ValueOf(process).Elem().FieldByName("handle")
	if !value.IsValid() {
		return 0
	}
	return uintptr(reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem().Uint())
}
