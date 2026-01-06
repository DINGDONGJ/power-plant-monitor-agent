package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"monitor-agent/monitor"
	"monitor-agent/provider"
	"monitor-agent/server"
	"monitor-agent/types"
)

func main() {
	var (
		pid            = flag.Int("pid", 0, "target process PID")
		name           = flag.String("name", "", "target process name")
		cpuThreshold   = flag.Float64("cpu-threshold", 80.0, "CPU threshold percentage")
		cpuExceedCount = flag.Int("cpu-exceed-count", 5, "consecutive CPU exceed count to trigger restart")
		restartCmd     = flag.String("restart-cmd", "", "command to restart the process")
		logFile        = flag.String("log-file", "", "JSONL log file path (auto-generated if empty)")
		addr           = flag.String("addr", ":8080", "HTTP server address")
	)
	flag.Parse()

	if *pid == 0 && *name == "" {
		log.Fatal("either -pid or -name is required")
	}

	// 生成日志文件名：logs/{进程名或pid}_{时间戳}.jsonl
	logFileName := *logFile
	if logFileName == "" {
		ts := time.Now().Format("20060102_150405")
		if *name != "" {
			logFileName = fmt.Sprintf("logs/%s_%s.jsonl", *name, ts)
		} else {
			logFileName = fmt.Sprintf("logs/pid%d_%s.jsonl", *pid, ts)
		}
		// 确保 logs 目录存在
		os.MkdirAll("logs", 0755)
	}

	cfg := types.MonitorConfig{
		PID:              int32(*pid),
		ProcessName:      *name,
		CPUThreshold:     *cpuThreshold,
		CPUExceedCount:   *cpuExceedCount,
		RestartCmd:       *restartCmd,
		MetricsBufferLen: 60,
		EventsBufferLen:  100,
		LogFile:          logFileName,
	}

	prov := provider.New()
	mon, err := monitor.New(cfg, prov)
	if err != nil {
		log.Fatalf("create monitor: %v", err)
	}

	if err := mon.Start(); err != nil {
		log.Fatalf("start monitor: %v", err)
	}
	status := mon.GetStatus()
	log.Printf("monitor started, target: pid=%d name=%s", status.TargetPID, cfg.ProcessName)

	srv := server.New(mon)
	go func() {
		log.Printf("HTTP server listening on %s", *addr)
		if err := http.ListenAndServe(*addr, srv); err != nil {
			log.Fatalf("http server: %v", err)
		}
	}()

	// 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("shutting down...")
	mon.Stop()
}
