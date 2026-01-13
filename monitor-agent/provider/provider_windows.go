//go:build windows

package provider

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

var (
	modkernel32               = syscall.NewLazyDLL("kernel32.dll")
	modpsapi                  = syscall.NewLazyDLL("psapi.dll")
	procGetProcessHandleCount = modkernel32.NewProc("GetProcessHandleCount")
	procOpenProcess           = modkernel32.NewProc("OpenProcess")
	procCloseHandle           = modkernel32.NewProc("CloseHandle")
	procGetProcessMemoryInfo  = modpsapi.NewProc("GetProcessMemoryInfo")
)

const (
	PROCESS_QUERY_INFORMATION = 0x0400
	PROCESS_VM_READ           = 0x0010
)

// PROCESS_MEMORY_COUNTERS_EX 结构体
type processMemoryCountersEx struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr // 页面缓冲池
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr // 非页面缓冲池
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

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

// getProcessMemoryPools 获取进程内存池信息
func getProcessMemoryPools(pid int32) (pagedPool, nonPagedPool uint64) {
	handle, _, _ := procOpenProcess.Call(
		uintptr(PROCESS_QUERY_INFORMATION|PROCESS_VM_READ),
		0,
		uintptr(pid),
	)
	if handle == 0 {
		return 0, 0
	}
	defer procCloseHandle.Call(handle)

	var memCounters processMemoryCountersEx
	memCounters.CB = uint32(unsafe.Sizeof(memCounters))

	ret, _, _ := procGetProcessMemoryInfo.Call(
		handle,
		uintptr(unsafe.Pointer(&memCounters)),
		uintptr(memCounters.CB),
	)
	if ret == 0 {
		return 0, 0
	}

	return uint64(memCounters.QuotaPagedPoolUsage), uint64(memCounters.QuotaNonPagedPoolUsage)
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
		// getMemoryPools: Windows 使用 GetProcessMemoryInfo API
		getProcessMemoryPools,
	)
}
