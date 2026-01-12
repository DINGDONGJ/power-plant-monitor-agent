//go:build windows

package service

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "MonitorAgent"
const serviceDisplayName = "电厂核心软件监视保障系统"
const serviceDescription = "监控系统进程，提供自愈和告警功能"

// WindowsService Windows 服务包装
type WindowsService struct {
	service *Service
}

// Execute 实现 svc.Handler 接口
func (ws *WindowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	if err := ws.service.Start(); err != nil {
		log.Printf("[SERVICE] Failed to start: %v", err)
		return true, 1
	}

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				ws.service.Stop()
				break loop
			default:
				log.Printf("[SERVICE] Unexpected control request #%d", c)
			}
		}
	}

	changes <- svc.Status{State: svc.Stopped}
	return false, 0
}

// RunAsService 以 Windows 服务方式运行
func RunAsService(cfg Config) error {
	// 检查是否在服务上下文中运行
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("failed to determine if running as service: %w", err)
	}

	s, err := New(cfg)
	if err != nil {
		return err
	}

	if isService {
		// 作为服务运行
		elog, err := eventlog.Open(serviceName)
		if err == nil {
			defer elog.Close()
			elog.Info(1, fmt.Sprintf("%s service starting", serviceName))
		}

		ws := &WindowsService{service: s}
		return svc.Run(serviceName, ws)
	}

	// 作为普通程序运行
	return runInteractive(s)
}

// runInteractive 交互式运行（非服务模式）
func runInteractive(s *Service) error {
	if err := s.Start(); err != nil {
		return err
	}

	log.Printf("[SERVICE] Running in interactive mode. Press Ctrl+C to stop.")
	s.Wait()
	return nil
}

// InstallService 安装 Windows 服务
func InstallService() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", serviceName)
	}

	// 获取配置文件路径
	configPath := filepath.Join(filepath.Dir(exePath), "config.json")

	s, err = m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName:  serviceDisplayName,
		Description:  serviceDescription,
		StartType:    mgr.StartAutomatic,
		ServiceStartName: "", // LocalSystem
	}, "-service", "-config", configPath)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()

	// 设置服务恢复选项（失败后自动重启）
	recoveryActions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}
	err = s.SetRecoveryActions(recoveryActions, 86400) // 24小时后重置计数
	if err != nil {
		log.Printf("[WARN] Failed to set recovery actions: %v", err)
	}

	// 创建事件日志源
	err = eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		log.Printf("[WARN] Failed to install event log: %v", err)
	}

	log.Printf("[SERVICE] Service %s installed successfully", serviceName)
	return nil
}

// UninstallService 卸载 Windows 服务
func UninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found: %w", serviceName, err)
	}
	defer s.Close()

	// 先停止服务
	status, err := s.Query()
	if err == nil && status.State != svc.Stopped {
		s.Control(svc.Stop)
		// 等待停止
		for i := 0; i < 10; i++ {
			time.Sleep(time.Second)
			status, err = s.Query()
			if err != nil || status.State == svc.Stopped {
				break
			}
		}
	}

	err = s.Delete()
	if err != nil {
		return fmt.Errorf("delete service: %w", err)
	}

	eventlog.Remove(serviceName)

	log.Printf("[SERVICE] Service %s uninstalled successfully", serviceName)
	return nil
}

// StartService 启动服务
func StartService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	err = s.Start()
	if err != nil {
		return fmt.Errorf("start service: %w", err)
	}

	log.Printf("[SERVICE] Service %s started", serviceName)
	return nil
}

// StopService 停止服务
func StopService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("stop service: %w", err)
	}

	// 等待停止
	for status.State != svc.Stopped {
		time.Sleep(time.Second)
		status, err = s.Query()
		if err != nil {
			break
		}
	}

	log.Printf("[SERVICE] Service %s stopped", serviceName)
	return nil
}

// ServiceStatus 获取服务状态
func ServiceStatus() (string, error) {
	m, err := mgr.Connect()
	if err != nil {
		return "", fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return "not installed", nil
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return "", fmt.Errorf("query service: %w", err)
	}

	switch status.State {
	case svc.Stopped:
		return "stopped", nil
	case svc.StartPending:
		return "starting", nil
	case svc.StopPending:
		return "stopping", nil
	case svc.Running:
		return "running", nil
	default:
		return fmt.Sprintf("unknown (%d)", status.State), nil
	}
}
