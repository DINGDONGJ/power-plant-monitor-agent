package main

import (
	"flag"
	"fmt"
	"log"

	"monitor-agent/service"
)

var version = "1.0.0"

func main() {
	var (
		addr         = flag.String("addr", ":8080", "HTTP server address")
		cpuThreshold = flag.Float64("cpu-threshold", 80.0, "CPU threshold percentage")
		cpuExceed    = flag.Int("cpu-exceed-count", 5, "consecutive CPU exceed count")
		logDir       = flag.String("log-dir", "", "log directory (default: ./logs)")
		
		// 服务管理命令
		runService   = flag.Bool("service", false, "run as service")
		install      = flag.Bool("install", false, "install as system service")
		uninstall    = flag.Bool("uninstall", false, "uninstall system service")
		start        = flag.Bool("start", false, "start the service")
		stop         = flag.Bool("stop", false, "stop the service")
		status       = flag.Bool("status", false, "show service status")
		showVersion  = flag.Bool("version", false, "show version")
	)
	flag.Parse()

	// 显示版本
	if *showVersion {
		fmt.Printf("Monitor Agent v%s\n", version)
		return
	}

	// 服务管理命令
	if *install {
		if err := service.InstallService(); err != nil {
			log.Fatalf("Install failed: %v", err)
		}
		fmt.Println("Service installed successfully")
		return
	}

	if *uninstall {
		if err := service.UninstallService(); err != nil {
			log.Fatalf("Uninstall failed: %v", err)
		}
		fmt.Println("Service uninstalled successfully")
		return
	}

	if *start {
		if err := service.StartService(); err != nil {
			log.Fatalf("Start failed: %v", err)
		}
		return
	}

	if *stop {
		if err := service.StopService(); err != nil {
			log.Fatalf("Stop failed: %v", err)
		}
		return
	}

	if *status {
		s, err := service.ServiceStatus()
		if err != nil {
			log.Fatalf("Status check failed: %v", err)
		}
		fmt.Printf("Service status: %s\n", s)
		return
	}

	// 配置
	cfg := service.Config{
		Addr:           *addr,
		CPUThreshold:   *cpuThreshold,
		CPUExceedCount: *cpuExceed,
		LogDir:         *logDir,
	}

	// 运行服务
	if *runService {
		// 以服务模式运行
		if err := service.RunAsService(cfg); err != nil {
			log.Fatalf("Service error: %v", err)
		}
	} else {
		// 交互式运行
		runInteractive(cfg)
	}
}

func runInteractive(cfg service.Config) {
	s, err := service.New(cfg)
	if err != nil {
		log.Fatalf("Create service failed: %v", err)
	}

	if err := s.Start(); err != nil {
		log.Fatalf("Start failed: %v", err)
	}

	fmt.Println("Monitor Agent running in interactive mode")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Printf("Open http://localhost%s in browser\n", cfg.Addr)

	// 等待信号
	waitForSignal()
	
	s.Stop()
}
