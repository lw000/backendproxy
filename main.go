package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"backendproxy/config"
	"backendproxy/logger"
	"backendproxy/proxy"
	"backendproxy/service"

	"go.uber.org/zap"
)

var (
	configFile = flag.String("config", "config.toml", "配置文件路径")
)

func main() {
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configFile)
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	if err := logger.Init(cfg.Log); err != nil {
		fmt.Printf("初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	zap.L().Info("启动代理转发服务",
		zap.Int("proxy_count", len(cfg.Proxies)),
		zap.Bool("monitor_enabled", cfg.Monitor.Enabled),
	)

	// 创建代理管理器
	proxyManager, err := proxy.NewManager(cfg)
	if err != nil {
		zap.L().Fatal("创建代理管理器失败", zap.Error(err))
	}

	// 创建服务
	svc := service.New(proxyManager)

	// 启动服务
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		zap.L().Fatal("启动服务失败", zap.Error(err))
	}

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	zap.L().Info("接收到关闭信号，停止服务...")

	// 停止服务
	if err := svc.Stop(); err != nil {
		zap.L().Error("停止服务失败", zap.Error(err))
	}

	zap.L().Info("服务已停止")
}
