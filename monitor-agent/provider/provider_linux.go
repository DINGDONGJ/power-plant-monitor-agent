//go:build linux

package provider

import (
	"fmt"
	"os/exec"

	"github.com/shirou/gopsutil/v3/process"
	"monitor-agent/types"
)

type linuxProvider struct{}

func New() ProcProvider {
	return &linuxProvider{}
}

func (p *linuxProvider) FindAllPIDsByName(name string) ([]int32, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	var pids []int32
	for _, proc := range procs {
		n, _ := proc.Name()
		if n == name {
			pids = append(pids, proc.Pid)
		}
	}
	return pids, nil
}

func (p *linuxProvider) FindPIDByName(name string) (int32, error) {
	pids, err := p.FindAllPIDsByName(name)
	if err != nil {
		return 0, err
	}
	if len(pids) == 0 {
		return 0, fmt.Errorf("process %s not found", name)
	}
	if len(pids) > 1 {
		return 0, fmt.Errorf("multiple processes found with name %s: %v, please use -pid to specify", name, pids)
	}
	return pids[0], nil
}

func (p *linuxProvider) GetMetrics(pid int32) (*types.ProcessMetrics, error) {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}
	cpu, _ := proc.CPUPercent()
	mem, _ := proc.MemoryInfo()
	name, _ := proc.Name()

	var rss uint64
	if mem != nil {
		rss = mem.RSS
	}
	return &types.ProcessMetrics{
		PID:      pid,
		Name:     name,
		CPUPct:   cpu,
		RSSBytes: rss,
		Alive:    true,
	}, nil
}

func (p *linuxProvider) IsAlive(pid int32) bool {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return false
	}
	running, _ := proc.IsRunning()
	return running
}

func (p *linuxProvider) KillProcess(pid int32) error {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func (p *linuxProvider) ExecuteRestart(cmd string) error {
	return exec.Command("sh", "-c", cmd).Start()
}

func (p *linuxProvider) ListAllProcesses() ([]types.ProcessInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	var result []types.ProcessInfo
	for _, proc := range procs {
		name, _ := proc.Name()
		cpu, _ := proc.CPUPercent()
		mem, _ := proc.MemoryInfo()
		status, _ := proc.Status()

		var rss uint64
		if mem != nil {
			rss = mem.RSS
		}
		statusStr := ""
		if len(status) > 0 {
			statusStr = status[0]
		}
		result = append(result, types.ProcessInfo{
			PID:      proc.Pid,
			Name:     name,
			CPUPct:   cpu,
			RSSBytes: rss,
			Status:   statusStr,
		})
	}
	return result, nil
}
