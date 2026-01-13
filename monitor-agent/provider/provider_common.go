package provider

import (
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"monitor-agent/types"
)

// 磁盘 IO 采样状态（增强版）
type ioSample struct {
	readBytes  uint64
	writeBytes uint64
	readCount  uint64
	writeCount uint64
	sampleTime time.Time
	// 上次计算的速率
	lastReadRate  float64
	lastWriteRate float64
	lastReadOps   float64
	lastWriteOps  float64
}

// 网络采样状态
type netSample struct {
	bytesRecv  uint64
	bytesSent  uint64
	sampleTime time.Time
	recvRate   float64
	sendRate   float64
}

// commonProvider 通用 provider 实现
type commonProvider struct {
	ioSamplesMu sync.RWMutex
	ioSamples   map[int32]*ioSample

	// 系统指标缓存（后台 goroutine 更新）
	sysCPUMu      sync.RWMutex
	sysCPUPercent float64

	// 网络采样
	netSampleMu sync.RWMutex
	netSample   *netSample

	// 平台特定函数
	matchProcessName func(procName, targetName string) bool
	executeCommand   func(cmd string) error
	formatCmdline    func(exe string) string
	getHandleCount   func(pid int32) int32                        // 可选，Windows 专用
	getMemoryPools   func(pid int32) (pagedPool, nonPagedPool uint64) // 可选，Windows 专用
}

// newCommonProvider 创建通用 provider
func newCommonProvider(
	matchName func(procName, targetName string) bool,
	execCmd func(cmd string) error,
	fmtCmdline func(exe string) string,
	getHandles func(pid int32) int32,
	getMemPools func(pid int32) (uint64, uint64),
) *commonProvider {
	p := &commonProvider{
		ioSamples:        make(map[int32]*ioSample),
		matchProcessName: matchName,
		executeCommand:   execCmd,
		formatCmdline:    fmtCmdline,
		getHandleCount:   getHandles,
		getMemoryPools:   getMemPools,
	}
	go p.sampleSystemMetrics()
	return p
}

// sampleSystemMetrics 后台定时采集系统 CPU 和网络
func (p *commonProvider) sampleSystemMetrics() {
	for {
		// CPU 采样
		cpuPercent, _ := cpu.Percent(time.Second, false)
		if len(cpuPercent) > 0 {
			p.sysCPUMu.Lock()
			p.sysCPUPercent = cpuPercent[0]
			p.sysCPUMu.Unlock()
		}

		// 网络采样
		p.sampleNetwork()
	}
}

// sampleNetwork 采集网络流量
func (p *commonProvider) sampleNetwork() {
	counters, err := net.IOCounters(false) // false = 汇总所有网卡
	if err != nil || len(counters) == 0 {
		return
	}

	now := time.Now()
	totalRecv := counters[0].BytesRecv
	totalSent := counters[0].BytesSent

	p.netSampleMu.Lock()
	defer p.netSampleMu.Unlock()

	if p.netSample == nil {
		p.netSample = &netSample{
			bytesRecv:  totalRecv,
			bytesSent:  totalSent,
			sampleTime: now,
		}
		return
	}

	deltaTime := now.Sub(p.netSample.sampleTime).Seconds()
	if deltaTime > 0.1 {
		p.netSample.recvRate = float64(totalRecv-p.netSample.bytesRecv) / deltaTime
		p.netSample.sendRate = float64(totalSent-p.netSample.bytesSent) / deltaTime
		p.netSample.bytesRecv = totalRecv
		p.netSample.bytesSent = totalSent
		p.netSample.sampleTime = now
	}
}

func (p *commonProvider) FindAllPIDsByName(name string) ([]int32, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	var pids []int32
	for _, proc := range procs {
		n, _ := proc.Name()
		if p.matchProcessName(n, name) {
			pids = append(pids, proc.Pid)
		}
	}
	return pids, nil
}

func (p *commonProvider) FindPIDByName(name string) (int32, error) {
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

func (p *commonProvider) GetMetrics(pid int32) (*types.ProcessMetrics, error) {
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

func (p *commonProvider) IsAlive(pid int32) bool {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return false
	}
	running, _ := proc.IsRunning()
	return running
}

func (p *commonProvider) KillProcess(pid int32) error {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func (p *commonProvider) ExecuteRestart(cmd string) error {
	return p.executeCommand(cmd)
}

// calcDiskIO 计算磁盘 IO 速率和次数
func (p *commonProvider) calcDiskIO(pid int32, readBytes, writeBytes, readCount, writeCount uint64) (readRate, writeRate, readOps, writeOps float64) {
	now := time.Now()

	p.ioSamplesMu.Lock()
	defer p.ioSamplesMu.Unlock()

	sample, exists := p.ioSamples[pid]
	if !exists {
		p.ioSamples[pid] = &ioSample{
			readBytes:  readBytes,
			writeBytes: writeBytes,
			readCount:  readCount,
			writeCount: writeCount,
			sampleTime: now,
		}
		return 0, 0, 0, 0
	}

	deltaTime := now.Sub(sample.sampleTime).Seconds()
	if deltaTime < 0.1 {
		return sample.lastReadRate, sample.lastWriteRate, sample.lastReadOps, sample.lastWriteOps
	}

	readRate = float64(readBytes-sample.readBytes) / deltaTime
	writeRate = float64(writeBytes-sample.writeBytes) / deltaTime
	readOps = float64(readCount-sample.readCount) / deltaTime
	writeOps = float64(writeCount-sample.writeCount) / deltaTime

	sample.readBytes = readBytes
	sample.writeBytes = writeBytes
	sample.readCount = readCount
	sample.writeCount = writeCount
	sample.sampleTime = now
	sample.lastReadRate = readRate
	sample.lastWriteRate = writeRate
	sample.lastReadOps = readOps
	sample.lastWriteOps = writeOps

	return readRate, writeRate, readOps, writeOps
}

func (p *commonProvider) ListAllProcesses() ([]types.ProcessInfo, error) {
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
		createTime, _ := proc.CreateTime()
		
		// 获取句柄数/文件描述符数
		var numFDs int32
		if p.getHandleCount != nil {
			// Windows: 使用平台特定的 GetProcessHandleCount
			numFDs = p.getHandleCount(proc.Pid)
		} else {
			// Linux: 使用 gopsutil 的 NumFDs
			numFDs, _ = proc.NumFDs()
		}

		// 如果 cmdline 为空，尝试获取可执行文件路径
		if cmdline == "" {
			if exe, err := proc.Exe(); err == nil && exe != "" {
				cmdline = p.formatCmdline(exe)
			}
		}

		var rss, vms uint64
		if memInfo != nil {
			rss = memInfo.RSS
			vms = memInfo.VMS
		}
		statusStr := ""
		if len(status) > 0 {
			statusStr = status[0]
		}

		// 获取内存池信息
		var pagedPool, nonPagedPool uint64
		if p.getMemoryPools != nil {
			// Windows: 使用平台特定的 GetProcessMemoryInfo
			pagedPool, nonPagedPool = p.getMemoryPools(proc.Pid)
		} else if memInfo != nil {
			// Linux: 近似处理
			// 非页面池：使用 Data 段（数据段，常驻内存）
			// 如果 Data 为 0，使用 RSS - Shared 作为近似
			nonPagedPool = memInfo.Data
			if nonPagedPool == 0 {
				// 使用 RSS 的一部分作为近似
				nonPagedPool = memInfo.RSS / 10 // 约 10% 作为非页面池近似
			}
			// 页面池：使用 Swap（交换空间）
			// 如果 Swap 为 0，使用 VMS - RSS 作为近似（虚拟内存中未驻留的部分）
			pagedPool = memInfo.Swap
			if pagedPool == 0 && memInfo.VMS > memInfo.RSS {
				pagedPool = (memInfo.VMS - memInfo.RSS) / 10 // 约 10% 作为页面池近似
			}
		}

		// 计算磁盘 IO 速率和次数
		var diskIO, diskReadRate, diskWriteRate, diskReadOps, diskWriteOps float64
		if ioCounters != nil {
			diskReadRate, diskWriteRate, diskReadOps, diskWriteOps = p.calcDiskIO(
				proc.Pid,
				ioCounters.ReadBytes, ioCounters.WriteBytes,
				ioCounters.ReadCount, ioCounters.WriteCount,
			)
			diskIO = diskReadRate + diskWriteRate // 兼容旧字段
		}

		// 计算已运行时间（秒）
		var uptime int64
		if createTime > 0 {
			uptime = (time.Now().UnixMilli() - createTime) / 1000
		}

		result = append(result, types.ProcessInfo{
			PID:           proc.Pid,
			Name:          name,
			CPUPct:        cpuPct,
			RSSBytes:      rss,
			VMS:           vms,
			PagedPool:     pagedPool,
			NonPagedPool:  nonPagedPool,
			Status:        statusStr,
			Username:      username,
			NumFDs:        numFDs,
			DiskIO:        diskIO,
			DiskReadRate:  diskReadRate,
			DiskWriteRate: diskWriteRate,
			DiskReadOps:   diskReadOps,
			DiskWriteOps:  diskWriteOps,
			Uptime:        uptime,
			Cmdline:       cmdline,
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

func (p *commonProvider) GetSystemMetrics() (*types.SystemMetrics, error) {
	memInfo, _ := mem.VirtualMemory()

	p.sysCPUMu.RLock()
	cpuPct := p.sysCPUPercent
	p.sysCPUMu.RUnlock()

	// 获取网络流量
	p.netSampleMu.RLock()
	var netRecv, netSent uint64
	var netRecvRate, netSendRate float64
	if p.netSample != nil {
		netRecv = p.netSample.bytesRecv
		netSent = p.netSample.bytesSent
		netRecvRate = p.netSample.recvRate
		netSendRate = p.netSample.sendRate
	}
	p.netSampleMu.RUnlock()

	return &types.SystemMetrics{
		CPUPercent:    cpuPct,
		MemoryTotal:   memInfo.Total,
		MemoryUsed:    memInfo.Used,
		MemoryPercent: float64(memInfo.Used) / float64(memInfo.Total) * 100,
		NetBytesRecv:  netRecv,
		NetBytesSent:  netSent,
		NetRecvRate:   netRecvRate,
		NetSendRate:   netSendRate,
	}, nil
}
