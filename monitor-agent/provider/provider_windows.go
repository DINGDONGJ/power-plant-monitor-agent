//go:build windows

package provider

import (
	"fmt"
	"os/exec"
)

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
	)
}
