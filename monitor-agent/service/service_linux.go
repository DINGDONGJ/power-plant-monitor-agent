//go:build linux

package service

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

const serviceName = "monitor-agent"
const serviceDisplayName = "电厂核心软件监视保障系统"

// RunAsService 以服务方式运行（Linux 使用 systemd）
func RunAsService(cfg Config) error {
	s, err := New(cfg)
	if err != nil {
		return err
	}

	if err := s.Start(); err != nil {
		return err
	}

	// 监听系统信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-sigCh
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Printf("[SERVICE] Received signal %v, shutting down...", sig)
			s.Stop()
			return nil
		case syscall.SIGHUP:
			log.Printf("[SERVICE] Received SIGHUP, reloading configuration...")
			// TODO: 重新加载配置
		}
	}
}

// InstallService 安装 systemd 服务
func InstallService() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// 获取绝对路径
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("get absolute path: %w", err)
	}

	workDir := filepath.Dir(exePath)

	serviceContent := fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
Type=simple
ExecStart=%s -service
WorkingDirectory=%s
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=%s

[Install]
WantedBy=multi-user.target
`, serviceDisplayName, exePath, workDir, serviceName)

	servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", serviceName)
	
	err = os.WriteFile(servicePath, []byte(serviceContent), 0644)
	if err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	log.Printf("[SERVICE] Service file created at %s", servicePath)
	log.Printf("[SERVICE] Run the following commands to enable and start the service:")
	log.Printf("  sudo systemctl daemon-reload")
	log.Printf("  sudo systemctl enable %s", serviceName)
	log.Printf("  sudo systemctl start %s", serviceName)

	return nil
}

// UninstallService 卸载 systemd 服务
func UninstallService() error {
	servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", serviceName)

	// 检查文件是否存在
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		return fmt.Errorf("service file not found: %s", servicePath)
	}

	err := os.Remove(servicePath)
	if err != nil {
		return fmt.Errorf("remove service file: %w", err)
	}

	log.Printf("[SERVICE] Service file removed: %s", servicePath)
	log.Printf("[SERVICE] Run 'sudo systemctl daemon-reload' to apply changes")

	return nil
}

// StartService 启动服务（通过 systemctl）
func StartService() error {
	log.Printf("[SERVICE] To start the service, run:")
	log.Printf("  sudo systemctl start %s", serviceName)
	return nil
}

// StopService 停止服务（通过 systemctl）
func StopService() error {
	log.Printf("[SERVICE] To stop the service, run:")
	log.Printf("  sudo systemctl stop %s", serviceName)
	return nil
}

// ServiceStatus 获取服务状态
func ServiceStatus() (string, error) {
	servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", serviceName)
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		return "not installed", nil
	}
	
	log.Printf("[SERVICE] To check service status, run:")
	log.Printf("  sudo systemctl status %s", serviceName)
	return "installed (check with systemctl)", nil
}
