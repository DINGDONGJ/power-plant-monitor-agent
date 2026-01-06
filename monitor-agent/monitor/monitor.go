package monitor

import (
	"fmt"
	"log"
	"sync"
	"time"

	"monitor-agent/buffer"
	"monitor-agent/logger"
	"monitor-agent/provider"
	"monitor-agent/types"
)

type Monitor struct {
	mu             sync.RWMutex
	config         types.MonitorConfig
	provider       provider.ProcProvider
	metricsBuffer  *buffer.RingBuffer[types.ProcessMetrics]
	eventsBuffer   *buffer.RingBuffer[types.Event]
	logger         *logger.JSONLLogger
	targetPID      int32
	running        bool
	stopCh         chan struct{}
	cpuExceedCnt   int
	lastMetric     *types.ProcessMetrics
	lastRestart    time.Time // 上次重启时间，用于冷却
	waitingRestart bool      // 正在等待新进程启动
}

func New(cfg types.MonitorConfig, prov provider.ProcProvider) (*Monitor, error) {
	var jsonlLogger *logger.JSONLLogger
	var err error
	if cfg.LogFile != "" {
		jsonlLogger, err = logger.NewJSONLLogger(cfg.LogFile)
		if err != nil {
			return nil, fmt.Errorf("create logger: %w", err)
		}
	}
	if cfg.MetricsBufferLen == 0 {
		cfg.MetricsBufferLen = 60
	}
	if cfg.EventsBufferLen == 0 {
		cfg.EventsBufferLen = 100
	}
	return &Monitor{
		config:        cfg,
		provider:      prov,
		metricsBuffer: buffer.NewRingBuffer[types.ProcessMetrics](cfg.MetricsBufferLen),
		eventsBuffer:  buffer.NewRingBuffer[types.Event](cfg.EventsBufferLen),
		logger:        jsonlLogger,
		stopCh:        make(chan struct{}),
	}, nil
}

func (m *Monitor) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("monitor already running")
	}
	// 解析目标 PID
	if m.config.PID > 0 {
		m.targetPID = m.config.PID
	} else if m.config.ProcessName != "" {
		pid, err := m.provider.FindPIDByName(m.config.ProcessName)
		if err != nil {
			m.mu.Unlock()
			return fmt.Errorf("find process: %w", err)
		}
		m.targetPID = pid
	} else {
		m.mu.Unlock()
		return fmt.Errorf("pid or process_name required")
	}
	m.running = true
	m.mu.Unlock()

	go m.loop()
	return nil
}

func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	close(m.stopCh)
	m.running = false
	if m.logger != nil {
		m.logger.Close()
	}
}

func (m *Monitor) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.collect()
		}
	}
}

func (m *Monitor) collect() {
	m.mu.Lock()
	pid := m.targetPID
	m.mu.Unlock()

	alive := m.provider.IsAlive(pid)
	var metric types.ProcessMetrics
	metric.Timestamp = time.Now()
	metric.PID = pid
	metric.Alive = alive

	if alive {
		if met, err := m.provider.GetMetrics(pid); err == nil {
			metric = *met
			metric.Timestamp = time.Now()
			metric.Alive = true
		}
	}

	m.metricsBuffer.Push(metric)
	m.mu.Lock()
	m.lastMetric = &metric
	m.mu.Unlock()

	if m.logger != nil {
		m.logger.Write(metric)
	}

	m.checkRules(metric)
}

func (m *Monitor) checkRules(metric types.ProcessMetrics) {
	// 正在等待新进程启动，跳过规则检测
	m.mu.RLock()
	waiting := m.waitingRestart
	m.mu.RUnlock()
	if waiting {
		return
	}

	// 进程退出检测
	if !metric.Alive {
		evt := types.Event{
			Timestamp: time.Now(),
			Type:      "exit",
			PID:       metric.PID,
			Name:      metric.Name,
			Message:   "process exited",
		}
		m.addEvent(evt)
		m.triggerRestart("process exited")
		return
	}

	// CPU 连续超阈值检测
	if m.config.CPUThreshold > 0 && metric.CPUPct > m.config.CPUThreshold {
		m.cpuExceedCnt++
		if m.cpuExceedCnt >= m.config.CPUExceedCount {
			evt := types.Event{
				Timestamp: time.Now(),
				Type:      "cpu_threshold",
				PID:       metric.PID,
				Name:      metric.Name,
				Message:   fmt.Sprintf("CPU %.2f%% exceeded threshold %.2f%% for %d times", metric.CPUPct, m.config.CPUThreshold, m.cpuExceedCnt),
			}
			m.addEvent(evt)
			m.triggerRestart("CPU threshold exceeded")
			m.cpuExceedCnt = 0
		}
	} else {
		m.cpuExceedCnt = 0
	}
}

func (m *Monitor) addEvent(evt types.Event) {
	m.eventsBuffer.Push(evt)
	if m.logger != nil {
		m.logger.Write(evt)
	}
	log.Printf("[EVENT] %s: %s (pid=%d)", evt.Type, evt.Message, evt.PID)
}

func (m *Monitor) triggerRestart(reason string) {
	if m.config.RestartCmd == "" {
		log.Printf("[WARN] no restart command configured")
		return
	}
	// 重启冷却：10秒内不重复触发
	m.mu.Lock()
	if time.Since(m.lastRestart) < 10*time.Second {
		m.mu.Unlock()
		log.Printf("[INFO] restart skipped (cooling down)")
		return
	}
	m.lastRestart = time.Now()
	pid := m.targetPID
	m.mu.Unlock()

	// 先杀掉旧进程（如果还活着）
	if m.provider.IsAlive(pid) {
		log.Printf("[ACTION] killing old process pid=%d", pid)
		if err := m.provider.KillProcess(pid); err != nil {
			log.Printf("[WARN] kill process failed: %v", err)
		} else {
			log.Printf("[INFO] old process killed")
		}
		// 等待进程完全退出
		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("[ACTION] triggering restart: %s", reason)
	if err := m.provider.ExecuteRestart(m.config.RestartCmd); err != nil {
		log.Printf("[ERROR] restart failed: %v", err)
		return
	}
	evt := types.Event{
		Timestamp: time.Now(),
		Type:      "restart",
		PID:       pid,
		Message:   fmt.Sprintf("restart triggered: %s", reason),
	}
	m.addEvent(evt)

	// 按进程名监控时，等待新进程启动后重新查找 PID
	// 按 PID 监控时，进程死后停止监控（无法确定新进程）
	if m.config.ProcessName != "" {
		m.mu.Lock()
		m.waitingRestart = true
		m.mu.Unlock()
		go m.waitAndRefindPID()
	} else {
		log.Printf("[INFO] monitoring by PID, stopping after restart (cannot track new process)")
		m.mu.Lock()
		m.waitingRestart = true // 停止规则检测，不再触发重启
		m.mu.Unlock()
	}
}

func (m *Monitor) waitAndRefindPID() {
	defer func() {
		m.mu.Lock()
		m.waitingRestart = false
		m.mu.Unlock()
	}()

	time.Sleep(2 * time.Second)
	for i := 0; i < 10; i++ {
		pids, err := m.provider.FindAllPIDsByName(m.config.ProcessName)
		if err == nil && len(pids) > 0 {
			// 取第一个找到的（通常是最新启动的）
			m.mu.Lock()
			m.targetPID = pids[0]
			m.mu.Unlock()
			log.Printf("[INFO] found new process pid=%d", pids[0])
			return
		}
		time.Sleep(time.Second)
	}
	log.Printf("[WARN] failed to find new process after restart")
}

// GetRecentMetrics 获取最近 n 条指标
func (m *Monitor) GetRecentMetrics(n int) []types.ProcessMetrics {
	return m.metricsBuffer.GetRecent(n)
}

// GetRecentEvents 获取最近 n 条事件
func (m *Monitor) GetRecentEvents(n int) []types.Event {
	return m.eventsBuffer.GetRecent(n)
}

// GetStatus 获取当前状态
func (m *Monitor) GetStatus() types.StatusResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return types.StatusResponse{
		Running:       m.running,
		TargetPID:     m.targetPID,
		TargetName:    m.config.ProcessName,
		CurrentMetric: m.lastMetric,
		Config:        m.config,
	}
}

// IsRunning 检查监控是否运行中
func (m *Monitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}
