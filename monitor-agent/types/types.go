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

// ProcessInfo 系统进程信息（用于列表展示）
type ProcessInfo struct {
	PID           int32   `json:"pid"`
	Name          string  `json:"name"`
	CPUPct        float64 `json:"cpu_pct"`
	RSSBytes      uint64  `json:"rss_bytes"`
	VMS           uint64  `json:"vms"`             // 虚拟内存大小
	PagedPool     uint64  `json:"paged_pool"`      // 页面缓冲池
	NonPagedPool  uint64  `json:"non_paged_pool"`  // 非页面缓冲池
	Status        string  `json:"status"`
	Username      string  `json:"username"`        // 发布者/用户
	NumFDs        int32   `json:"num_fds"`         // 句柄数/文件描述符数
	DiskIO        float64 `json:"disk_io"`         // 磁盘速率 (B/s) - 保留兼容
	DiskReadRate  float64 `json:"disk_read_rate"`  // 磁盘读取速率 (B/s)
	DiskWriteRate float64 `json:"disk_write_rate"` // 磁盘写入速率 (B/s)
	DiskReadOps   float64 `json:"disk_read_ops"`   // 磁盘读取次数/秒
	DiskWriteOps  float64 `json:"disk_write_ops"`  // 磁盘写入次数/秒
	Uptime        int64   `json:"uptime"`          // 已运行时间（秒）
	Cmdline       string  `json:"cmdline"`         // 命令行
}

// MonitorTarget 监控目标
type MonitorTarget struct {
	PID             int32   `json:"pid"`
	Name            string  `json:"name"`                        // 进程名
	Alias           string  `json:"alias,omitempty"`             // 备注名称（如：电力监控主进程）
	Cmdline         string  `json:"cmdline,omitempty"`           // 进程命令行（用于自动填充重启命令）
	RestartCmd      string  `json:"restart_cmd,omitempty"`       // 重启命令
	AutoRestart     bool    `json:"auto_restart"`                // 退出时自动重启
	CPUThreshold    float64 `json:"cpu_threshold,omitempty"`     // CPU阈值 (%)
	MemThreshold    uint64  `json:"mem_threshold,omitempty"`     // 内存阈值 (bytes)
	CPUExceedCount  int     `json:"cpu_exceed_count,omitempty"`  // CPU连续超限次数触发
	MemExceedCount  int     `json:"mem_exceed_count,omitempty"`  // 内存连续超限次数触发
	RestartCooldown int     `json:"restart_cooldown,omitempty"`  // 重启冷却时间（秒）
}

// MultiMonitorConfig 多进程监控配置
type MultiMonitorConfig struct {
	Targets          []MonitorTarget `json:"targets"`
	CPUThreshold     float64         `json:"cpu_threshold"`
	CPUExceedCount   int             `json:"cpu_exceed_count"`
	SampleInterval   int             `json:"sample_interval"` // 采样间隔（秒）
	MetricsBufferLen int             `json:"metrics_buffer_len"`
	EventsBufferLen  int             `json:"events_buffer_len"`
	LogDir           string          `json:"log_dir"`
}

// SystemMetrics 系统指标
type SystemMetrics struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryTotal   uint64  `json:"memory_total"`
	MemoryUsed    uint64  `json:"memory_used"`
	MemoryPercent float64 `json:"memory_percent"`
	// 网络流量
	NetBytesRecv uint64  `json:"net_bytes_recv"` // 网络接收总字节
	NetBytesSent uint64  `json:"net_bytes_sent"` // 网络发送总字节
	NetRecvRate  float64 `json:"net_recv_rate"`  // 接收速率 (B/s)
	NetSendRate  float64 `json:"net_send_rate"`  // 发送速率 (B/s)
}
