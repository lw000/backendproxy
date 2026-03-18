package monitor

import (
	"backendproxy/proxy"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Monitor 监控服务
type Monitor struct {
	proxyManager *proxy.Manager
	port         int
	engine       *gin.Engine
	server       *http.Server
}

// New 创建监控服务
func New(proxyManager *proxy.Manager, port int) *Monitor {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	m := &Monitor{
		proxyManager: proxyManager,
		port:         port,
		engine:       engine,
	}

	m.setupRoutes()

	return m
}

// setupRoutes 设置路由
func (m *Monitor) setupRoutes() {
	m.engine.Static("/static", "./static")
	m.engine.GET("/", m.indexHandler)
	m.engine.GET("/api/status", m.statusHandler)
	m.engine.GET("/api/proxy/:port", m.proxyDetailHandler)
	m.engine.GET("/api/logs/:port", m.logsHandler)
}

// Start 启动监控服务
func (m *Monitor) Start() error {
	m.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", m.port),
		Handler: m.engine,
	}

	go func() {
		zap.L().Info("监控服务已启动", zap.Int("port", m.port))
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Error("监控服务异常退出", zap.Error(err))
		}
	}()

	return nil
}

// Stop 停止监控服务
func (m *Monitor) Stop() error {
	if m.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return m.server.Shutdown(ctx)
	}
	return nil
}

// indexHandler 首页处理器
func (m *Monitor) indexHandler(c *gin.Context) {
	c.File("./static/index.html")
}

// statusHandler 状态接口
func (m *Monitor) statusHandler(c *gin.Context) {
	statuses := m.proxyManager.GetStatus()
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": statuses,
	})
}

// proxyDetailHandler 代理详情接口
func (m *Monitor) proxyDetailHandler(c *gin.Context) {
	port := c.Param("port")
	var portNum int
	if _, err := fmt.Sscanf(port, "%d", &portNum); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"msg":  "无效的端口号",
		})
		return
	}

	status := m.proxyManager.GetStatus()
	for _, s := range status {
		// 检查端口（通过 JSON 反射）
		if statusMap, ok := getPortFromStatus(s); ok && statusMap == portNum {
			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"data": s,
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{
		"code": -1,
		"msg":  "代理服务不存在",
	})
}

// logsHandler 日志接口
func (m *Monitor) logsHandler(c *gin.Context) {
	port := c.Param("port")
	var portNum int
	if _, err := fmt.Sscanf(port, "%d", &portNum); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"msg":  "无效的端口号",
		})
		return
	}

	status := m.proxyManager.GetStatus()
	for _, s := range status {
		if statusMap, ok := getPortFromStatus(s); ok && statusMap == portNum {
			if logs, ok := getLogsFromStatus(s); ok {
				c.JSON(http.StatusOK, gin.H{
					"code": 0,
					"data": logs,
				})
				return
			}
		}
	}

	c.JSON(http.StatusNotFound, gin.H{
		"code": -1,
		"msg":  "代理服务不存在",
	})
}

// getPortFromStatus 从状态中提取端口号
func getPortFromStatus(status interface{}) (int, bool) {
	switch s := status.(type) {
	case *proxy.ProxyStatus:
		return s.Port, true
	case *proxy.TCPProxyStatus:
		return s.Port, true
	default:
		return 0, false
	}
}

// getLogsFromStatus 从状态中提取日志
func getLogsFromStatus(status interface{}) (interface{}, bool) {
	switch s := status.(type) {
	case *proxy.ProxyStatus:
		return s.RecentLogs, true
	case *proxy.TCPProxyStatus:
		return s.RecentLogs, true
	default:
		return nil, false
	}
}
