package provider

import "monitor-agent/types"

// ProcProvider 进程信息提供者接口，封装平台差异
type ProcProvider interface {
	// FindPIDByName 根据进程名查找 PID
	FindPIDByName(name string) (int32, error)
	// FindAllPIDsByName 根据进程名查找所有匹配的 PID
	FindAllPIDsByName(name string) ([]int32, error)
	// GetMetrics 获取进程指标
	GetMetrics(pid int32) (*types.ProcessMetrics, error)
	// IsAlive 检查进程是否存活
	IsAlive(pid int32) bool
	// KillProcess 杀死进程
	KillProcess(pid int32) error
	// ExecuteRestart 执行重启命令
	ExecuteRestart(cmd string) error
}
