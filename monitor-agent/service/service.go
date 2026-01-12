package service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"monitor-agent/monitor"
	"monitor-agent/provider"
	"monitor-agent/server"
	"monitor-agent/types"
)

// Config 服务配置
type Config struct {
	Addr           string
	CPUThreshold   float64
	CPUExceedCount int
	LogDir         string
	ConfigFile     string
}

// Service 监控服务
type Service struct {
	config     Config
	mm         *monitor.MultiMonitor
	httpServer *http.Server
	ctx        context.Context
	cancel     context.CancelFunc
}

// New 创建服务实例
func New(cfg Config) (*Service, error) {
	// 确保日志目录存在
	if cfg.LogDir == "" {
		exe, _ := os.Executable()
		cfg.LogDir = filepath.Join(filepath.Dir(exe), "logs")
	}
	os.MkdirAll(cfg.LogDir, 0755)

	// 设置日志输出到文件
	logFile := filepath.Join(cfg.LogDir, "service.log")
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		log.SetOutput(f)
	}

	monitorCfg := types.MultiMonitorConfig{
		CPUThreshold:     cfg.CPUThreshold,
		CPUExceedCount:   cfg.CPUExceedCount,
		SampleInterval:   1,
		MetricsBufferLen: 300,
		EventsBufferLen:  100,
		LogDir:           cfg.LogDir,
	}

	prov := provider.New()
	mm, err := monitor.NewMultiMonitor(monitorCfg, prov)
	if err != nil {
		return nil, fmt.Errorf("create multi monitor: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Service{
		config: cfg,
		mm:     mm,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Start 启动服务
func (s *Service) Start() error {
	log.Printf("[SERVICE] Starting monitor service...")
	log.Printf("[SERVICE] HTTP address: %s", s.config.Addr)
	log.Printf("[SERVICE] Log directory: %s", s.config.LogDir)

	// 启动 HTTP 服务器
	webSrv := server.NewWebServer(s.mm)
	s.httpServer = &http.Server{
		Addr:    s.config.Addr,
		Handler: webSrv,
	}

	go func() {
		log.Printf("[SERVICE] HTTP server listening on %s", s.config.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[SERVICE] HTTP server error: %v", err)
		}
	}()

	// 自动启动监控（如果有保存的配置）
	s.loadSavedTargets()

	log.Printf("[SERVICE] Service started successfully")
	return nil
}

// Stop 停止服务
func (s *Service) Stop() error {
	log.Printf("[SERVICE] Stopping monitor service...")

	// 保存当前监控目标
	s.saveTargets()

	// 停止监控
	s.mm.Stop()

	// 关闭 HTTP 服务器
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			log.Printf("[SERVICE] HTTP server shutdown error: %v", err)
		}
	}

	s.cancel()
	log.Printf("[SERVICE] Service stopped")
	return nil
}

// Wait 等待服务结束
func (s *Service) Wait() {
	<-s.ctx.Done()
}

// loadSavedTargets 加载保存的监控目标
func (s *Service) loadSavedTargets() {
	// TODO: 从配置文件加载保存的监控目标
	// 目前暂不实现持久化
}

// saveTargets 保存监控目标
func (s *Service) saveTargets() {
	// TODO: 保存监控目标到配置文件
	// 目前暂不实现持久化
}
