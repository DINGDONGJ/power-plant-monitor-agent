//go:build windows

package provider

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
	"monitor-agent/types"
)

// 磁盘 IO 采样状态
type ioSample struct {
	totalBytes uint64
	sampleTime time.Time
	lastRate   float64
}

type windowsProvider struct {
	ioSamplesMu sync.RWMutex
	ioSamples   map[int32]*ioSample
}

func New() ProcProvider {
	return &windowsProvider{
		ioSamples: make(map[int32]*ioSample),
	}
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
	cpuPct, _ := proc.CPUPercent()
	memInfo, _ := proc.MemoryInfo()
	name, _ := proc.Name()

	var rss uint64
	if memInfo != nil {
		rss = memInfo.RSS
	}
	return &types.ProcessMetrics{
		PID:      pid,
		Name:     name,
		CPUPct:   cpuPct,
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

// calcDiskIORate 计算磁盘 IO 速率 (B/s)
func (p *windowsProvider) calcDiskIORate(pid int32, currentTotal uint64) float64 {
	now := time.Now()

	p.ioSamplesMu.Lock()
	defer p.ioSamplesMu.Unlock()

	sample, exists := p.ioSamples[pid]
	if !exists {
		p.ioSamples[pid] = &ioSample{
			totalBytes: currentTotal,
			sampleTime: now,
			lastRate:   0,
		}
		return 0
	}

	deltaTime := now.Sub(sample.sampleTime).Seconds()
	if deltaTime < 0.5 {
		return sample.lastRate
	}

	deltaBytes := currentTotal - sample.totalBytes
	rate := float64(deltaBytes) / deltaTime

	sample.totalBytes = currentTotal
	sample.sampleTime = now
	sample.lastRate = rate

	return rate
}

func (p *windowsProvider) ListAllProcesses() ([]types.ProcessInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	alivePids := make(map[int32]bool)
	var result []types.ProcessInfo

	for _, proc := range procs {
		alivePids[proc.Pid] = true

		name, _ := proc.Name()
		cpuPct, _ := proc.CPUPercent()
		memInfo, _ := proc.MemoryInfo()
		status, _ := proc.Status()
		username, _ := proc.Username()
		cmdline, _ := proc.Cmdline()
		ioCounters, _ := proc.IOCounters()

		var rss uint64
		if memInfo != nil {
			rss = memInfo.RSS
		}
		statusStr := ""
		if len(status) > 0 {
			statusStr = status[0]
		}

		// 计算磁盘 IO 速率
		var diskIO float64
		if ioCounters != nil {
			totalIO := ioCounters.ReadBytes + ioCounters.WriteBytes
			diskIO = p.calcDiskIORate(proc.Pid, totalIO)
		}

		result = append(result, types.ProcessInfo{
			PID:      proc.Pid,
			Name:     name,
			CPUPct:   cpuPct,
			RSSBytes: rss,
			Status:   statusStr,
			Username: username,
			Cmdline:  cmdline,
			DiskIO:   diskIO,
		})
	}

	// 清理已退出进程的采样数据
	p.ioSamplesMu.Lock()
	for pid := range p.ioSamples {
		if !alivePids[pid] {
			delete(p.ioSamples, pid)
		}
	}
	p.ioSamplesMu.Unlock()

	return result, nil
}

func (p *windowsProvider) GetSystemMetrics() (*types.SystemMetrics, error) {
	cpuPercent, _ := cpu.Percent(time.Second, false)
	memInfo, _ := mem.VirtualMemory()

	var cpuPct float64
	if len(cpuPercent) > 0 {
		cpuPct = cpuPercent[0]
	}

	return &types.SystemMetrics{
		CPUPercent:    cpuPct,
		MemoryTotal:   memInfo.Total,
		MemoryUsed:    memInfo.Used,
		MemoryPercent: float64(memInfo.Used) / float64(memInfo.Total) * 100,
	}, nil
}
