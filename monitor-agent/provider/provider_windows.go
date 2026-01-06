//go:build windows

package provider

import (
	"fmt"
	"os/exec"

	"github.com/shirou/gopsutil/v3/process"
	"monitor-agent/types"
)

type windowsProvider struct{}

func New() ProcProvider {
	return &windowsProvider{}
}

func (p *windowsProvider) FindAllPIDsByName(name string) ([]int32, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	var pids []int32
	for _, proc := range procs {
		n, _ := proc.Name()
		if n == name || n == name+".exe" {
			pids = append(pids, proc.Pid)
		}
	}
	return pids, nil
}

func (p *windowsProvider) FindPIDByName(name string) (int32, error) {
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

func (p *windowsProvider) GetMetrics(pid int32) (*types.ProcessMetrics, error) {
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

func (p *windowsProvider) IsAlive(pid int32) bool {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return false
	}
	running, _ := proc.IsRunning()
	return running
}

func (p *windowsProvider) KillProcess(pid int32) error {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func (p *windowsProvider) ExecuteRestart(cmd string) error {
	return exec.Command("cmd", "/C", cmd).Start()
}
