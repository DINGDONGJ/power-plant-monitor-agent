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
	)
}
