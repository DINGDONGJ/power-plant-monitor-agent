package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"monitor-agent/monitor"
	"monitor-agent/provider"
	"monitor-agent/server"
	"monitor-agent/types"
)

func main() {
	var (
		addr         = flag.String("addr", ":8080", "HTTP server address")
		cpuThreshold = flag.Float64("cpu-threshold", 80.0, "CPU threshold percentage")
		cpuExceed    = flag.Int("cpu-exceed-count", 5, "consecutive CPU exceed count")
		logDir       = flag.String("log-dir", "logs", "log directory")
	)
	flag.Parse()

	cfg := types.MultiMonitorConfig{
		CPUThreshold:     *cpuThreshold,
		CPUExceedCount:   *cpuExceed,
		SampleInterval:   1,
		MetricsBufferLen: 300,
		EventsBufferLen:  100,
		LogDir:           *logDir,
	}

	prov := provider.New()
	mm, err := monitor.NewMultiMonitor(cfg, prov)
	if err != nil {
		log.Fatalf("create multi monitor: %v", err)
	}

	srv := server.NewWebServer(mm)

	go func() {
		log.Printf("Web server listening on %s", *addr)
		log.Printf("Open http://localhost%s in browser", *addr)
		if err := http.ListenAndServe(*addr, srv); err != nil {
			log.Fatalf("http server: %v", err)
		}
	}()

	// 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("shutting down...")
	mm.Stop()
}
