# Monitor-Agent

跨平台进程监控代理，支持 Windows 和 Linux，按 PID 或进程名监控目标进程，自动采集指标并根据规则触发重启。

## 功能特性

- 按 PID 或进程名定位目标进程
- 每秒采集 CPU%、RSS 内存、存活状态
- 规则引擎：进程退出或 CPU 连续超阈值时触发重启
- HTTP API：健康检查、状态查询、指标/事件查询
- Ring Buffer 内存缓存 + JSONL 文件落盘
- 平台差异封装在 ProcProvider 接口

## 项目结构

```
monitor-agent/
├── buffer/ring.go           # 泛型环形缓冲区
├── logger/jsonl.go          # JSONL 日志写入
├── monitor/monitor.go       # 核心监控逻辑
├── provider/
│   ├── provider.go          # ProcProvider 接口
│   ├── provider_linux.go    # Linux 实现
│   └── provider_windows.go  # Windows 实现
├── server/server.go         # HTTP 服务
├── types/types.go           # 数据结构
├── main.go                  # 入口
└── go.mod
```

## 编译

### Windows

```cmd
cd monitor-agent
go build -o monitor-agent.exe .
```

### Linux

```bash
cd monitor-agent
go build -o monitor-agent .
```

### 交叉编译

```bash
# 在 Linux 上编译 Windows 版本
GOOS=windows GOARCH=amd64 go build -o monitor-agent.exe .

# 在 Windows 上编译 Linux 版本
set GOOS=linux
set GOARCH=amd64
go build -o monitor-agent .
```

## 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-pid` | 目标进程 PID | - |
| `-name` | 目标进程名 | - |
| `-cpu-threshold` | CPU 阈值 (%) | 80 |
| `-cpu-exceed-count` | 连续超阈值次数触发重启 | 5 |
| `-restart-cmd` | 重启命令 | - |
| `-log-file` | JSONL 日志文件路径（不指定则自动生成） | 自动生成 |
| `-addr` | HTTP 监听地址 | :8080 |

> 注意：`-pid` 和 `-name` 二选一，必须指定其中一个。

## 日志文件命名

不指定 `-log-file` 时，自动在 `logs/` 目录下生成文件：

| 监控方式 | 文件名格式 | 示例 |
|---------|-----------|------|
| 按进程名 | `logs/{进程名}_{时间戳}.jsonl` | `logs/Notepad.exe_20260106_135014.jsonl` |
| 按 PID | `logs/pid{PID}_{时间戳}.jsonl` | `logs/pid1234_20260106_135014.jsonl` |

时间戳格式：`YYYYMMDD_HHMMSS`

## 使用示例

### Windows

> **注意**：在 PowerShell 中执行当前目录下的程序需要加 `.\` 前缀，CMD 则不需要。

**PowerShell：**
```powershell
# 监控记事本进程
.\monitor-agent.exe -name notepad.exe -restart-cmd "start notepad.exe"

# 监控指定 PID
.\monitor-agent.exe -pid 1234 -cpu-threshold 90 -cpu-exceed-count 3

# 监控 nginx（假设已安装）
.\monitor-agent.exe -name nginx.exe -restart-cmd "nginx.exe -s reload" -addr :9090
```

**CMD：**
```cmd
# 监控记事本进程
monitor-agent.exe -name notepad.exe -restart-cmd "start notepad.exe"

# 监控指定 PID
monitor-agent.exe -pid 1234 -cpu-threshold 90 -cpu-exceed-count 3
```

### Linux

```bash
# 监控 python 脚本进程
./monitor-agent -name python3 -restart-cmd "python3 /opt/myapp/app.py &"

# 监控指定 PID，自定义阈值
./monitor-agent -pid 1234 -cpu-threshold 70 -cpu-exceed-count 10

# 后台运行
nohup ./monitor-agent -name myapp -restart-cmd "/opt/myapp/restart.sh" > /dev/null 2>&1 &
```

## HTTP API

### GET /health

健康检查

```bash
curl http://localhost:8080/health
```

响应：
```json
{"status": "ok"}
```

### GET /status

当前监控状态

```bash
curl http://localhost:8080/status
```

响应：
```json
{
  "running": true,
  "target_pid": 1234,
  "target_name": "python3",
  "current_metric": {
    "timestamp": "2026-01-06T10:00:00Z",
    "pid": 1234,
    "name": "python3",
    "cpu_pct": 5.2,
    "rss_bytes": 52428800,
    "alive": true
  },
  "config": {
    "pid": 0,
    "process_name": "python3",
    "cpu_threshold": 80,
    "cpu_exceed_count": 5,
    "restart_cmd": "python3 /opt/myapp/app.py &",
    "metrics_buffer_len": 60,
    "events_buffer_len": 100,
    "log_file": "logs/python3_20260106_100000.jsonl"
  }
}
```

### GET /metrics/recent

最近 N 条指标（默认 10 条）

```bash
curl "http://localhost:8080/metrics/recent?n=5"
```

响应：
```json
[
  {"timestamp": "2026-01-06T10:00:05Z", "pid": 1234, "name": "python3", "cpu_pct": 3.1, "rss_bytes": 52428800, "alive": true},
  {"timestamp": "2026-01-06T10:00:04Z", "pid": 1234, "name": "python3", "cpu_pct": 4.2, "rss_bytes": 52428800, "alive": true}
]
```

### GET /events/recent

最近 N 条事件（默认 10 条）

```bash
curl "http://localhost:8080/events/recent?n=5"
```

响应：
```json
[
  {"timestamp": "2026-01-06T09:55:00Z", "type": "exit", "pid": 1234, "name": "python3", "message": "process exited"},
  {"timestamp": "2026-01-06T09:55:01Z", "type": "restart", "pid": 1234, "name": "", "message": "restart triggered: process exited"}
]
```

## 事件类型

| 类型 | 说明 |
|------|------|
| `exit` | 进程退出 |
| `cpu_threshold` | CPU 连续超阈值 |
| `restart` | 触发重启命令 |

## 日志文件格式

JSONL 格式，每行一条 JSON 记录，包含指标记录和事件记录两种类型：

### 指标记录字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `timestamp` | string | ISO 8601 时间戳 |
| `pid` | int | 进程 PID |
| `name` | string | 进程名 |
| `cpu_pct` | float | CPU 使用率 (%) |
| `rss_bytes` | int | 常驻内存大小 (bytes) |
| `alive` | bool | 进程是否存活 |

### 事件记录字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `timestamp` | string | ISO 8601 时间戳 |
| `type` | string | 事件类型：`exit` / `cpu_threshold` / `restart` |
| `pid` | int | 进程 PID |
| `name` | string | 进程名 |
| `message` | string | 事件描述 |

### 示例

```jsonl
{"timestamp":"2026-01-06T13:50:14.123+08:00","pid":24296,"name":"Notepad.exe","cpu_pct":0.016,"rss_bytes":88035328,"alive":true}
{"timestamp":"2026-01-06T13:50:15.124+08:00","pid":24296,"name":"Notepad.exe","cpu_pct":0.012,"rss_bytes":88035328,"alive":true}
{"timestamp":"2026-01-06T13:55:00.000+08:00","type":"exit","pid":24296,"name":"Notepad.exe","message":"process exited"}
{"timestamp":"2026-01-06T13:55:00.100+08:00","type":"restart","pid":24296,"name":"","message":"restart triggered: process exited"}
```

## 架构说明

```
┌─────────────────────────────────────────────────────────┐
│                      main.go                            │
│                   (启动入口/信号处理)                     │
└─────────────────┬───────────────────────────────────────┘
                  │
    ┌─────────────┴─────────────┐
    ▼                           ▼
┌─────────┐              ┌─────────────┐
│ Monitor │◄────────────►│ HTTP Server │
│  Core   │              │  /health    │
│         │              │  /status    │
│ - 采集  │              │  /metrics   │
│ - 规则  │              │  /events    │
│ - 重启  │              └─────────────┘
└────┬────┘
     │
     ▼
┌─────────────────┐     ┌─────────────┐
│  ProcProvider   │     │ Ring Buffer │
│  (接口)         │     │ (内存缓存)   │
├─────────────────┤     └─────────────┘
│ Linux 实现      │
│ Windows 实现    │     ┌─────────────┐
└─────────────────┘     │ JSONL Logger│
                        │ (文件落盘)   │
                        └─────────────┘
```

## 注意事项

1. 需要足够权限读取目标进程信息（Linux 可能需要 root）
2. 重启命令需确保可执行且路径正确
3. 按进程名监控时，重启后会自动重新查找新 PID
4. Ring Buffer 默认保留最近 60 条指标、100 条事件
