package monitor

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"monitor-agent/buffer"
	"monitor-agent/provider"
	"monitor-agent/types"
)

// MultiMonitor 多进程监控器
type MultiMonitor struct {
	mu             sync.RWMutex
	provider       provider.ProcProvider
	targets        map[int32]*targetState // PID -> 状态
	metricsBuffers map[int32]*buffer.RingBuffer[types.ProcessMetrics]
	eventsBuffer   *buffer.RingBuffer[types.Event]
	config         types.MultiMonitorConfig
	running        bool
	stopCh         chan struct{}
	logFile        *os.File
}

type targetState struct {
	target       types.MonitorTarget
	cpuExceedCnt int
	lastRestart  time.Time
	lastMetric   *types.ProcessMetrics
	exitReported bool // 是否已报告退出事件
}

func NewMultiMonitor(cfg types.MultiMonitorConfig, prov provider.ProcProvider) (*MultiMonitor, error) {
	if cfg.SampleInterval <= 0 {
		cfg.SampleInterval = 1
	}
	if cfg.MetricsBufferLen <= 0 {
		cfg.MetricsBufferLen = 300 // 5分钟
	}
	if cfg.EventsBufferLen <= 0 {
		cfg.EventsBufferLen = 100
	}
	if cfg.LogDir == "" {
		cfg.LogDir = "logs"
	}
	os.MkdirAll(cfg.LogDir, 0755)

	// 创建日志文件
	logPath := fmt.Sprintf("%s/multi_monitor_%s.jsonl", cfg.LogDir, time.Now().Format("20060102_150405"))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	m := &MultiMonitor{
		provider:       prov,
		targets:        make(map[int32]*targetState),
		metricsBuffers: make(map[int32]*buffer.RingBuffer[types.ProcessMetrics]),
		eventsBuffer:   buffer.NewRingBuffer[types.Event](cfg.EventsBufferLen),
		config:         cfg,
		stopCh:         make(chan struct{}),
		logFile:        logFile,
	}

	return m, nil
}

// AddTarget 添加监控目标
func (m *MultiMonitor) AddTarget(target types.MonitorTarget) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.targets[target.PID]; exists {
		return fmt.Errorf("target PID %d already monitored", target.PID)
	}

	// 验证进程存在
	if !m.provider.IsAlive(target.PID) {
		return fmt.Errorf("process PID %d not found", target.PID)
	}

	// 立即获取一次指标
	var initialMetric *types.ProcessMetrics
	if met, err := m.provider.GetMetrics(target.PID); err == nil {
		met.Timestamp = time.Now()
		met.Alive = true
		initialMetric = met
	}

	state := &targetState{target: target, lastMetric: initialMetric}
	m.targets[target.PID] = state
	
	buf := buffer.NewRingBuffer[types.ProcessMetrics](m.config.MetricsBufferLen)
	if initialMetric != nil {
		buf.Push(*initialMetric)
	}
	m.metricsBuffers[target.PID] = buf

	log.Printf("[INFO] Added monitor target: PID=%d Name=%s", target.PID, target.Name)
	return nil
}

// RemoveTarget 移除监控目标
func (m *MultiMonitor) RemoveTarget(pid int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.targets, pid)
	delete(m.metricsBuffers, pid)
	log.Printf("[INFO] Removed monitor target: PID=%d", pid)
}

// GetTargets 获取所有监控目标
func (m *MultiMonitor) GetTargets() []types.MonitorTarget {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []types.MonitorTarget
	for _, state := range m.targets {
		result = append(result, state.target)
	}
	return result
}

// Start 启动监控
func (m *MultiMonitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	
	// 如果日志文件已关闭，重新创建
	if m.logFile == nil {
		logPath := fmt.Sprintf("%s/multi_monitor_%s.jsonl", m.config.LogDir, time.Now().Format("20060102_150405"))
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			m.logFile = f
		}
	}
	m.mu.Unlock()

	go m.loop()
	log.Printf("[INFO] MultiMonitor started")
}

// Stop 停止监控
func (m *MultiMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	m.running = false
	close(m.stopCh)
	m.stopCh = make(chan struct{}) // 重新创建 channel 以便下次启动
	if m.logFile != nil {
		m.logFile.Close()
		m.logFile = nil
	}
	log.Printf("[INFO] MultiMonitor stopped")
}

func (m *MultiMonitor) loop() {
	ticker := time.NewTicker(time.Duration(m.config.SampleInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.collectAll()
		}
	}
}

func (m *MultiMonitor) collectAll() {
	m.mu.Lock()
	pids := make([]int32, 0, len(m.targets))
	for pid := range m.targets {
		pids = append(pids, pid)
	}
	m.mu.Unlock()

	for _, pid := range pids {
		m.collectOne(pid)
	}
}

func (m *MultiMonitor) collectOne(pid int32) {
	m.mu.Lock()
	state, exists := m.targets[pid]
	if !exists {
		m.mu.Unlock()
		return
	}
	buf := m.metricsBuffers[pid]
	m.mu.Unlock()

	alive := m.provider.IsAlive(pid)
	metric := types.ProcessMetrics{
		Timestamp: time.Now(),
		PID:       pid,
		Alive:     alive,
	}

	if alive {
		if met, err := m.provider.GetMetrics(pid); err == nil {
			metric = *met
			metric.Timestamp = time.Now()
			metric.Alive = true
		}
		// 进程恢复运行，重置退出标记
		m.mu.Lock()
		state.exitReported = false
		m.mu.Unlock()
	}

	buf.Push(metric)
	m.mu.Lock()
	state.lastMetric = &metric
	exitReported := state.exitReported
	m.mu.Unlock()

	// 写入日志
	m.writeLog(metric)

	// 检查规则：只在首次检测到退出时报告
	if !alive && !exitReported {
		m.mu.Lock()
		state.exitReported = true
		m.mu.Unlock()
		
		evt := types.Event{
			Timestamp: time.Now(),
			Type:      "exit",
			PID:       pid,
			Name:      state.target.Name,
			Message:   "process exited",
		}
		m.addEvent(evt)
	}
}

func (m *MultiMonitor) writeLog(v any) {
	if m.logFile == nil {
		return
	}
	data, _ := json.Marshal(v)
	m.logFile.Write(append(data, '\n'))
}

func (m *MultiMonitor) addEvent(evt types.Event) {
	m.eventsBuffer.Push(evt)
	m.writeLog(evt)
	log.Printf("[EVENT] %s: %s (pid=%d)", evt.Type, evt.Message, evt.PID)
}

// GetMetrics 获取指定进程的最近指标
func (m *MultiMonitor) GetMetrics(pid int32, n int) []types.ProcessMetrics {
	m.mu.RLock()
	buf, exists := m.metricsBuffers[pid]
	m.mu.RUnlock()
	if !exists {
		return nil
	}
	return buf.GetRecent(n)
}

// GetAllLatestMetrics 获取所有监控目标的最新指标
func (m *MultiMonitor) GetAllLatestMetrics() map[int32]*types.ProcessMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[int32]*types.ProcessMetrics)
	for pid, state := range m.targets {
		if state.lastMetric != nil {
			result[pid] = state.lastMetric
		}
	}
	return result
}

// GetRecentEvents 获取最近事件
func (m *MultiMonitor) GetRecentEvents(n int) []types.Event {
	return m.eventsBuffer.GetRecent(n)
}

// IsRunning 检查是否运行中
func (m *MultiMonitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// ListAllProcesses 列出系统所有进程
func (m *MultiMonitor) ListAllProcesses() ([]types.ProcessInfo, error) {
	return m.provider.ListAllProcesses()
}
