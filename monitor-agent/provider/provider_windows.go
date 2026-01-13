//go:build windows

package provider

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

var (
	modkernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGetProcessHandleCount = modkernel32.NewProc("GetProcessHandleCount")
	procOpenProcess          = modkernel32.NewProc("OpenProcess")
	procCloseHandle          = modkernel32.NewProc("CloseHandle")
)

const (
	PROCESS_QUERY_INFORMATION = 0x0400
)

// getProcessHandleCount 获取进程句柄数
func getProcessHandleCount(pid int32) int32 {
	handle, _, _ := procOpenProcess.Call(
		uintptr(PROCESS_QUERY_INFORMATION),
		0,
		uintptr(pid),
	)
	if handle == 0 {
		return 0
	}
	defer procCloseHandle.Call(handle)

	var count uint32
	ret, _, _ := procGetProcessHandleCount.Call(handle, uintptr(unsafe.Pointer(&count)))
	if ret == 0 {
		return 0
	}
	return int32(count)
}

func New() ProcProvider {
	return newCommonProvider(
		// matchProcessName: Windows 需要匹配 .exe 后缀
		func(procName, targetName string) bool {
			return procName == targetName || procName == targetName+".exe"
		},
		// executeCommand: Windows 使用 cmd /C
		func(cmd string) error {
			return exec.Command("cmd", "/C", cmd).Start()
		},
		// formatCmdline: Windows 给路径加引号
		func(exe string) string {
			return fmt.Sprintf("\"%s\"", exe)
		},
		// getHandleCount: Windows 使用 GetProcessHandleCount API
		getProcessHandleCount,
	)
}
