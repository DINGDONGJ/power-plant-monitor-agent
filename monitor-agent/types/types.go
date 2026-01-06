package types

import "time"

// ProcessMetrics 进程指标
type ProcessMetrics struct {
	Timestamp time.Time `json:"timestamp"`
	PID       int32     `json:"pid"`
	Name      string    `json:"name"`
	CPUPct    float64   `json:"cpu_pct"`
	RSSBytes  uint64    `json:"rss_bytes"`
	Alive     bool      `json:"alive"`
}

// Event 事件记录
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "exit", "cpu_threshold", "restart"
	PID       int32     `json:"pid"`
	Name      string    `json:"name"`
	Message   string    `json:"message"`
}

// MonitorConfig 监控配置
type MonitorConfig struct {
	PID              int32   `json:"pid,omitempty"`
	ProcessName      string  `json:"process_name,omitempty"`
	CPUThreshold     float64 `json:"cpu_threshold"`
	CPUExceedCount   int     `json:"cpu_exceed_count"` // 连续超阈值次数
	RestartCmd       string  `json:"restart_cmd"`
	MetricsBufferLen int     `json:"metrics_buffer_len"`
	EventsBufferLen  int     `json:"events_buffer_len"`
	LogFile          string  `json:"log_file"`
}

// StatusResponse /status 接口响应
type StatusResponse struct {
	Running       bool           `json:"running"`
	TargetPID     int32          `json:"target_pid"`
	TargetName    string         `json:"target_name"`
	CurrentMetric *ProcessMetrics `json:"current_metric,omitempty"`
	Config        MonitorConfig  `json:"config"`
}
