//go:build linux

package provider

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

// CPU 采样状态（对齐 top 实现）
type cpuSample struct {
	procJiffies uint64  // 进程 CPU 时间 (utime + stime)
	sysTotal    uint64  // 采样时的系统总 jiffies
	lastPct     float64 // 上次计算的 CPU 百分比
}

type linuxProvider struct {
	ioSamplesMu  sync.RWMutex
	ioSamples    map[int32]*ioSample
	cpuSamplesMu sync.RWMutex
	cpuSamples   map[int32]*cpuSample

	// 系统 CPU 采样
	sysCPUMu      sync.RWMutex
	sysCPUTotal   uint64  // 当前系统总 jiffies
	sysCPUIdle    uint64  // 当前系统空闲 jiffies
	sysCPUPercent float64 // 系统 CPU 百分比
	lastSysTotal  uint64  // 上次系统总 jiffies
	lastSysIdle   uint64  // 上次系统空闲 jiffies

	// 系统参数
	numCPU int // CPU 核心数
}

func New() ProcProvider {
	numCPU, _ := cpu.Counts(true)
	if numCPU == 0 {
		numCPU = 1
	}

	p := &linuxProvider{
		ioSamples:  make(map[int32]*ioSample),
		cpuSamples: make(map[int32]*cpuSample),
		numCPU:     numCPU,
	}

	// 初始化系统 CPU 采样
	total, idle := p.readProcStat()
	p.sysCPUTotal = total
	p.sysCPUIdle = idle
	p.lastSysTotal = total
	p.lastSysIdle = idle

	// 启动后台 goroutine 采集系统 CPU
	go p.sampleSystemCPU()
	return p
}

// sampleSystemCPU 后台定时采集系统 CPU（读 /proc/stat）
func (p *linuxProvider) sampleSystemCPU() {
	for {
		time.Sleep(time.Second)

		total, idle := p.readProcStat()

		p.sysCPUMu.Lock()
		deltaTotal := total - p.lastSysTotal
		deltaIdle := idle - p.lastSysIdle
		if deltaTotal > 0 {
			p.sysCPUPercent = float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100
		}
		// 更新为本次采样值
		p.lastSysTotal = total
		p.lastSysIdle = idle
		p.sysCPUTotal = total
		p.sysCPUIdle = idle
		p.sysCPUMu.Unlock()
	}
}

// readProcStat 读取 /proc/stat 获取系统 CPU 时间
// 返回 total（所有 CPU 时间之和）和 idle（idle + iowait）
func (p *linuxProvider) readProcStat() (total, idle uint64) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			// cpu user nice system idle iowait irq softirq steal guest guest_nice
			// 索引: 0    1    2    3      4    5      6   7       8     9     10
			if len(fields) >= 6 {
				for i := 1; i < len(fields); i++ {
					v, _ := strconv.ParseUint(fields[i], 10, 64)
					total += v
				}
				// idle = idle + iowait（对齐 top 的空闲计算）
				idleVal, _ := strconv.ParseUint(fields[4], 10, 64)
				iowaitVal, _ := strconv.ParseUint(fields[5], 10, 64)
				idle = idleVal + iowaitVal
			}
		}
	}
	return total, idle
}

// readProcPidStat 读取 /proc/[pid]/stat 获取进程 CPU 时间
func (p *linuxProvider) readProcPidStat(pid int32) (procJiffies uint64, ok bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}

	// /proc/[pid]/stat 格式: pid (comm) state ppid ... utime stime ...
	// utime 是第 14 个字段，stime 是第 15 个字段（从 1 开始计数）
	// 需要处理 comm 中可能包含空格和括号的情况
	content := string(data)

	// 找到最后一个 ')' 的位置，之后的才是可靠的字段
	lastParen := strings.LastIndex(content, ")")
	if lastParen == -1 || lastParen+2 >= len(content) {
		return 0, false
	}

	// ')' 之后的字段从 state 开始（第 3 个字段）
	fields := strings.Fields(content[lastParen+2:])
	// utime 是第 14 个字段，在 ')' 之后是第 14-3+1 = 12 个（索引 11）
	// stime 是第 15 个字段，在 ')' 之后是第 15-3+1 = 13 个（索引 12）
	if len(fields) < 13 {
		return 0, false
	}

	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)
	return utime + stime, true
}

// calcCPUPercent 计算进程 CPU 使用率（对齐 top 实现）
// 公式: procCPU% = Δproc_jiffies / Δsys_total_jiffies * 100 * numCPU
func (p *linuxProvider) calcCPUPercent(pid int32) float64 {
	procJiffies, ok := p.readProcPidStat(pid)
	if !ok {
		return 0
	}

	// 实时读取系统 total jiffies，确保分子分母时间窗口对齐
	sysTotal, _ := p.readProcStat()

	p.cpuSamplesMu.Lock()
	defer p.cpuSamplesMu.Unlock()

	sample, exists := p.cpuSamples[pid]
	if !exists {
		p.cpuSamples[pid] = &cpuSample{
			procJiffies: procJiffies,
			sysTotal:    sysTotal,
			lastPct:     0,
		}
		return 0
	}

	deltaSysTotal := sysTotal - sample.sysTotal
	if deltaSysTotal == 0 {
		return sample.lastPct
	}

	deltaProc := procJiffies - sample.procJiffies

	// 对齐 top: procCPU% = Δproc / Δtotal * 100 * numCPU
	cpuPct := float64(deltaProc) / float64(deltaSysTotal) * 100 * float64(p.numCPU)

	sample.procJiffies = procJiffies
	sample.sysTotal = sysTotal
	sample.lastPct = cpuPct

	return cpuPct
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

	cpuPct := p.calcCPUPercent(pid)
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

func (p *linuxProvider) IsAlive(pid int32) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

func (p *linuxProvider) KillProcess(pid int32) error {
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		return err
	}
	return proc.Kill()
}

func (p *linuxProvider) ExecuteRestart(cmd string) error {
	return exec.Command("sh", "-c", cmd).Start()
}

// calcDiskIORate 计算磁盘 IO 速率 (B/s)
func (p *linuxProvider) calcDiskIORate(pid int32, currentTotal uint64) float64 {
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
	if deltaTime < 0.1 {
		return sample.lastRate
	}

	deltaBytes := currentTotal - sample.totalBytes
	rate := float64(deltaBytes) / deltaTime

	sample.totalBytes = currentTotal
	sample.sampleTime = now
	sample.lastRate = rate

	return rate
}

func (p *linuxProvider) ListAllProcesses() ([]types.ProcessInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	alivePids := make(map[int32]bool)
	var result []types.ProcessInfo

	for _, proc := range procs {
		alivePids[proc.Pid] = true

		name, _ := proc.Name()
		cpuPct := p.calcCPUPercent(proc.Pid)
		memInfo, _ := proc.MemoryInfo()
		status, _ := proc.Status()
		username, _ := proc.Username()
		cmdline, _ := proc.Cmdline()
		ioCounters, _ := proc.IOCounters()
		createTime, _ := proc.CreateTime() // 毫秒时间戳

		// 如果 cmdline 为空，尝试获取可执行文件路径
		if cmdline == "" {
			if exe, err := proc.Exe(); err == nil && exe != "" {
				cmdline = exe
			}
		}

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

		// 计算已运行时间（秒）
		var uptime int64
		if createTime > 0 {
			uptime = (time.Now().UnixMilli() - createTime) / 1000
		}

		result = append(result, types.ProcessInfo{
			PID:      proc.Pid,
			Name:     name,
			CPUPct:   cpuPct,
			RSSBytes: rss,
			Status:   statusStr,
			Username: username,
			DiskIO:   diskIO,
			Uptime:   uptime,
			Cmdline:  cmdline,
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

	p.cpuSamplesMu.Lock()
	for pid := range p.cpuSamples {
		if !alivePids[pid] {
			delete(p.cpuSamples, pid)
		}
	}
	p.cpuSamplesMu.Unlock()

	return result, nil
}

func (p *linuxProvider) GetSystemMetrics() (*types.SystemMetrics, error) {
	memInfo, _ := mem.VirtualMemory()

	// 从缓存读取系统 CPU
	p.sysCPUMu.RLock()
	cpuPct := p.sysCPUPercent
	p.sysCPUMu.RUnlock()

	return &types.SystemMetrics{
		CPUPercent:    cpuPct,
		MemoryTotal:   memInfo.Total,
		MemoryUsed:    memInfo.Used,
		MemoryPercent: float64(memInfo.Used) / float64(memInfo.Total) * 100,
	}, nil
}
