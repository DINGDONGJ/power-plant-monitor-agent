//go:build linux

package provider

import (
	"os/exec"
)

func New() ProcProvider {
	return newCommonProvider(
		// matchProcessName: Linux 直接匹配
		func(procName, targetName string) bool {
			return procName == targetName
		},
		// executeCommand: Linux 使用 sh -c
		func(cmd string) error {
			return exec.Command("sh", "-c", cmd).Start()
		},
		// formatCmdline: Linux 直接返回
		func(exe string) string {
			return exe
		},
		// getHandleCount: Linux 使用 gopsutil 的 NumFDs (返回 nil 使用默认实现)
		nil,
		// getMemoryPools: Linux 无直接对应，使用 gopsutil 的 Data 段近似
		nil,
	)
}
