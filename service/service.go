package service

import (
	"context"
	"backendproxy/monitor"
	"backendproxy/proxy"

	"go.uber.org/zap"
)

// Service 服务接口
type Service interface {
	Start(ctx context.Context) error
	Stop() error
}

// ProxyService 代理服务实现
type ProxyService struct {
	proxyManager *proxy.Manager
	monitor      *monitor.Monitor
}

// New 创建服务
func New(proxyManager *proxy.Manager) *ProxyService {
	return &ProxyService{
		proxyManager: proxyManager,
	}
}

// Start 启动服务
func (s *ProxyService) Start(ctx context.Context) error {
	// 启动代理服务
	if err := s.proxyManager.Start(ctx); err != nil {
		return err
	}

	// 启动监控服务（如果启用）
	cfg := s.proxyManager.GetConfig()
	if cfg.Monitor.Enabled {
		s.monitor = monitor.New(s.proxyManager, cfg.Monitor.Port)
		if err := s.monitor.Start(); err != nil {
			zap.L().Error("启动监控服务失败", zap.Error(err))
		}
	}

	zap.L().Info("所有服务启动完成")
	return nil
}

// Stop 停止服务
func (s *ProxyService) Stop() error {
	// 停止监控服务
	if s.monitor != nil {
		if err := s.monitor.Stop(); err != nil {
			zap.L().Error("停止监控服务失败", zap.Error(err))
		}
	}

	// 停止代理服务
	if err := s.proxyManager.Stop(); err != nil {
		return err
	}

	zap.L().Info("所有服务已停止")
	return nil
}
