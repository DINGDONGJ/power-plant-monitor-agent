# 电厂核心软件监视保障系统

一个轻量级的进程监控代理系统，提供实时进程监控、自愈功能和 Web 管理界面。支持 Windows 和 Linux 双平台，可部署为系统服务实现 7×24 小时无人值守运行。

## 项目背景

在电厂等关键基础设施环境中，核心软件的稳定运行至关重要。本系统旨在：
- 实时监控关键进程的运行状态
- 在进程异常退出时自动重启（自愈）
- 在资源占用超限时及时告警
- 提供直观的 Web 界面进行管理和查看

## 功能特性

### 进程监控
- 实时采集系统所有进程的 CPU、内存、磁盘 IO 等指标
- 支持按进程名、PID、用户等条件搜索过滤
- 同名进程自动分组，支持展开/折叠查看
- 可自定义显示列，支持拖拽调整列顺序

### 自愈功能
- **退出自动重启**：监控的进程退出后自动执行重启命令
- **CPU 阈值监控**：CPU 占用连续超限后触发告警或重启
- **内存阈值监控**：内存占用连续超限后触发告警或重启
- **重启冷却时间**：防止频繁重启，可配置冷却间隔
- **重启命令自动填充**：根据进程命令行自动生成重启命令

### Web 界面
- 终端风格的黑绿配色，专业感强
- 系统资源实时曲线图（CPU/内存），彩虹呼吸动画效果
- 三个功能面板：进程列表、监控面板、事件日志
- 响应式设计，支持各种屏幕尺寸

### 系统服务
- 支持 Windows Service 部署
- 支持 Linux systemd 部署
- 开机自动启动
- 崩溃自动恢复
- 优雅关闭，正确处理系统信号

## 系统架构

```
monitor-agent/
├── cmd/web/              # 主程序入口
│   ├── main.go           # 命令行参数处理
│   ├── signal_windows.go # Windows 信号处理
│   └── signal_linux.go   # Linux 信号处理
├── monitor/              # 监控核心逻辑
│   ├── monitor.go        # 单进程监控器
│   └── multi_monitor.go  # 多进程监控器（自愈逻辑）
├── provider/             # 系统指标采集
│   ├── provider.go       # 接口定义
│   ├── provider_windows.go # Windows 实现
│   └── provider_linux.go   # Linux 实现
├── server/               # HTTP 服务
│   ├── web_server.go     # API 路由和处理
│   └── static/           # 静态资源（嵌入到二进制）
│       └── index.html    # Web 界面
├── service/              # 系统服务支持
│   ├── service.go        # 服务核心逻辑
│   ├── service_windows.go # Windows Service 实现
│   └── service_linux.go    # Linux systemd 实现
├── buffer/               # 数据结构
│   └── ring.go           # 泛型环形缓冲区
├── logger/               # 日志记录
│   └── jsonl.go          # JSONL 格式日志
├── types/                # 数据类型定义
│   └── types.go          # 结构体定义
└── logs/                 # 日志输出目录
```

## 技术栈

### 后端语言
- **Go 1.19+**：高性能、跨平台编译、原生并发支持、单文件部署

### 核心依赖

| 包名 | 版本 | 用途 |
|------|------|------|
| `github.com/shirou/gopsutil/v3` | v3.23.12 | 跨平台系统信息采集（进程、CPU、内存、磁盘 IO） |
| `golang.org/x/sys` | v0.15.0 | Windows Service API 支持 |

### gopsutil 间接依赖

| 包名 | 用途 |
|------|------|
| `github.com/go-ole/go-ole` | Windows OLE/COM 接口 |
| `github.com/yusufpapurcu/wmi` | Windows WMI 查询 |
| `github.com/tklauser/go-sysconf` | 系统配置读取 |
| `github.com/tklauser/numcpus` | CPU 核心数检测 |
| `github.com/lufia/plan9stats` | Plan9 系统支持 |
| `github.com/power-devops/perfstat` | AIX 系统性能统计 |
| `github.com/shoenig/go-m1cpu` | Apple M1 芯片支持 |

### 前端技术
- **HTML5/CSS3**：页面结构和样式，终端风格设计
- **原生 JavaScript (ES6+)**：无框架依赖，轻量高效
- **Canvas API**：实时绘制 CPU/内存时间序列曲线图
- **Fetch API**：异步请求后端 RESTful API
- **LocalStorage**：保存用户列配置偏好

### 数据格式
- **JSON**：API 请求响应格式
- **JSONL**：日志文件格式（每行一个 JSON 对象，便于流式处理）

### 系统集成
- **Windows Service API**：通过 `golang.org/x/sys/windows/svc` 实现
- **Linux systemd**：生成标准 `.service` 单元文件
- **embed**：Go 1.16+ 静态资源嵌入，单文件部署

## 快速开始

### 环境要求
- Go 1.19 或更高版本
- Windows 7+ 或 Linux（支持 systemd）

### 编译

```bash
# 克隆项目
git clone <repository-url>
cd monitor-agent

# 编译 Windows 版本
go build -o monitor-web.exe ./cmd/web

# 编译 Linux 版本
GOOS=linux go build -o monitor-web ./cmd/web

# 交叉编译 Linux ARM64（如树莓派）
GOOS=linux GOARCH=arm64 go build -o monitor-web-arm64 ./cmd/web
```

### 运行

#### 交互式运行（开发测试）

```bash
# Windows
.\monitor-web.exe

# Linux
./monitor-web
```

启动后访问 http://localhost:8080

#### 自定义参数

```bash
# 指定端口和日志目录
.\monitor-web.exe -addr :9090 -log-dir C:\logs

# 设置 CPU 阈值
.\monitor-web.exe -cpu-threshold 90 -cpu-exceed-count 10
```

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-addr` | HTTP 服务监听地址 | `:8080` |
| `-cpu-threshold` | 全局 CPU 阈值百分比 | `80` |
| `-cpu-exceed-count` | CPU 连续超限触发次数 | `5` |
| `-log-dir` | 日志文件目录 | `./logs` |
| `-service` | 以服务模式运行 | `false` |
| `-install` | 安装为系统服务 | - |
| `-uninstall` | 卸载系统服务 | - |
| `-start` | 启动服务 | - |
| `-stop` | 停止服务 | - |
| `-status` | 查看服务状态 | - |
| `-version` | 显示版本号 | - |

## 服务部署

### Windows 服务

以管理员身份运行 PowerShell：

```powershell
# 1. 安装服务
.\monitor-web.exe -install
# 输出: Service installed successfully

# 2. 启动服务
.\monitor-web.exe -start
# 或在 services.msc 中启动 "电厂核心软件监视保障系统"

# 3. 查看状态
.\monitor-web.exe -status
# 输出: Service status: running

# 4. 停止服务
.\monitor-web.exe -stop

# 5. 卸载服务
.\monitor-web.exe -uninstall
```

服务特性：
- 服务名称：`MonitorAgent`
- 显示名称：`电厂核心软件监视保障系统`
- 启动类型：自动（开机自启）
- 恢复选项：失败后自动重启（5秒、10秒、30秒递增）

### Linux systemd

```bash
# 1. 安装服务（需要 root 权限）
sudo ./monitor-web -install
# 生成 /etc/systemd/system/monitor-agent.service

# 2. 重新加载 systemd 配置
sudo systemctl daemon-reload

# 3. 启用开机自启
sudo systemctl enable monitor-agent

# 4. 启动服务
sudo systemctl start monitor-agent

# 5. 查看状态
sudo systemctl status monitor-agent

# 6. 查看实时日志
sudo journalctl -u monitor-agent -f

# 7. 停止服务
sudo systemctl stop monitor-agent

# 8. 卸载服务
sudo ./monitor-web -uninstall
sudo systemctl daemon-reload
```

## Web 界面使用

### 进程列表

1. **查看进程**：显示系统所有进程，每 2 秒自动刷新
2. **搜索过滤**：在搜索框输入关键字过滤进程
3. **排序**：点击列标题排序，再次点击切换升序/降序
4. **分组**：同名进程自动折叠，点击展开查看详情
5. **列管理**：
   - 拖拽列标题调整顺序
   - 右键列标题显示/隐藏列
6. **添加监控**：勾选进程后点击"添加到监控"

### 监控面板

1. **查看目标**：显示所有监控中的进程及其实时指标
2. **配置自愈**：点击进程卡片上的 ⚙ 按钮
   - 退出时自动重启：开启后进程退出会自动执行重启命令
   - 重启命令：默认使用进程原始命令行，可自定义
   - CPU 阈值：设置 CPU 占用告警阈值（0 表示不监控）
   - 内存阈值：设置内存占用告警阈值（0 表示不监控）
   - 超限触发次数：连续 N 次超限后才触发
   - 重启冷却时间：两次重启之间的最小间隔
3. **启动/停止**：控制监控采样的运行状态
4. **移除目标**：单个移除或全部移除

### 事件日志

记录所有监控事件：
- `exit`：进程退出
- `restart`：执行重启命令
- `cpu_threshold`：CPU 超限
- `mem_threshold`：内存超限

## API 接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/processes` | GET | 获取所有进程列表 |
| `/api/system` | GET | 获取系统 CPU/内存指标 |
| `/api/monitor/targets` | GET | 获取监控目标列表 |
| `/api/monitor/add` | POST | 添加监控目标 |
| `/api/monitor/remove` | POST | 移除监控目标 |
| `/api/monitor/removeAll` | POST | 移除所有目标 |
| `/api/monitor/update` | POST | 更新目标配置 |
| `/api/monitor/start` | POST | 启动监控 |
| `/api/monitor/stop` | POST | 停止监控 |
| `/api/metrics/latest` | GET | 获取最新指标 |
| `/api/events` | GET | 获取事件日志 |
| `/api/status` | GET | 获取监控状态 |

## 日志文件

日志保存在 `logs/` 目录：

| 文件 | 说明 |
|------|------|
| `service.log` | 服务运行日志 |
| `multi_monitor_*.jsonl` | 监控数据（JSONL 格式） |

JSONL 日志示例：
```json
{"timestamp":"2026-01-08T18:00:00Z","pid":1234,"name":"app.exe","cpu_pct":5.2,"rss_bytes":104857600,"alive":true}
{"timestamp":"2026-01-08T18:00:01Z","type":"exit","pid":1234,"name":"app.exe","message":"进程已退出"}
```

## 常见问题

### Q: 服务安装失败？
A: 确保以管理员身份运行（Windows）或使用 sudo（Linux）。

### Q: 重启命令不生效？
A: 检查重启命令是否正确，可以先在终端手动测试。对于 GUI 程序，可能需要使用完整路径。

### Q: CPU/内存显示不准确？
A: 系统使用 gopsutil 库采集数据，与任务管理器可能有细微差异，属于正常现象。

### Q: 如何监控远程服务器？
A: 在远程服务器上部署本程序，通过 `http://<服务器IP>:8080` 访问。注意配置防火墙规则。

## 许可证

MIT License
