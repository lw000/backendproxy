package proxy

import (
	"backendproxy/config"
	"backendproxy/logger"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// RequestLog 请求日志
type RequestLog struct {
	Timestamp   time.Time `json:"timestamp"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	StatusCode  int       `json:"status_code"`
	Latency     int64     `json:"latency"` // 毫秒
	RequestData string    `json:"request_data,omitempty"`
	ResponseData string   `json:"response_data,omitempty"`
}

// ProxyStatus 代理状态（HTTP）
type ProxyStatus struct {
	Port          int            `json:"port"`
	Label         string         `json:"label"`
	Type          string         `json:"type"`
	Target        string         `json:"target"`
	Running       bool           `json:"running"`
	TotalReqs     int64          `json:"total_reqs"`
	SuccessReqs   int64          `json:"success_reqs"`
	FailedReqs    int64          `json:"failed_reqs"`
	AvgLatency    int64          `json:"avg_latency"` // 毫秒
	RecentLogs    []RequestLog   `json:"recent_logs"`
	TotalLatency  int64          `json:"total_latency"`
	StartTime     time.Time      `json:"start_time"`
}

// ProxyService 代理服务
type ProxyService struct {
	config      config.ProxyConfig
	reverseProxy *httputil.ReverseProxy
	status      *ProxyStatus
	mu          sync.RWMutex
	server      *http.Server
}

// Manager 代理管理器
type Manager struct {
	proxies    map[int]*ProxyService
	tcpProxies map[int]*TCPProxyService
	mu         sync.RWMutex // HTTP 代理锁
	tcpMu      sync.Mutex   // TCP 代理锁
	cfg        *config.Config
}

// GetConfig 获取配置
func (m *Manager) GetConfig() *config.Config {
	return m.cfg
}

// NewManager 创建代理管理器
func NewManager(cfg *config.Config) (*Manager, error) {
	mgr := &Manager{
		proxies:    make(map[int]*ProxyService),
		tcpProxies: make(map[int]*TCPProxyService),
		cfg:        cfg,
	}

	for _, proxyCfg := range cfg.Proxies {
		if err := mgr.addProxy(proxyCfg); err != nil {
			return nil, err
		}
	}

	return mgr, nil
}

// addProxy 添加代理服务
func (m *Manager) addProxy(cfg config.ProxyConfig) error {
	// 默认类型为 http
	if cfg.Type == "" {
		cfg.Type = "http"
	}

	switch cfg.Type {
	case "tcp":
		// TCP 代理不需要预先创建，在 start 时创建
		return nil
	case "http":
		target, err := url.Parse(cfg.Target)
		if err != nil {
			return fmt.Errorf("解析目标地址失败 [%s]: %w", cfg.Target, err)
		}

		reverseProxy := httputil.NewSingleHostReverseProxy(target)

		// 自定义错误处理
		reverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			logger.L().Error("代理请求失败",
				zap.Int("port", cfg.Port),
				zap.String("label", cfg.Label),
				zap.String("path", r.URL.Path),
				zap.Error(err),
			)
			w.WriteHeader(http.StatusBadGateway)
		}

		// 自定义响应处理器
		originalDirector := reverseProxy.Director
		reverseProxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Header.Set("X-Forwarded-Host", req.Host)
			req.Header.Set("X-Forwarded-Proto", req.URL.Scheme)
		}

		service := &ProxyService{
			config: cfg,
			reverseProxy: reverseProxy,
			status: &ProxyStatus{
				Port:       cfg.Port,
				Label:      cfg.Label,
				Type:       "http",
				Target:     cfg.Target,
				Running:    false,
				RecentLogs: make([]RequestLog, 0, 100),
			},
		}

		m.mu.Lock()
		m.proxies[cfg.Port] = service
		m.mu.Unlock()

		logger.L().Info("添加 HTTP 代理服务",
			zap.Int("port", cfg.Port),
			zap.String("label", cfg.Label),
			zap.String("target", cfg.Target),
		)

		return nil
	default:
		return fmt.Errorf("不支持的代理类型: %s", cfg.Type)
	}
}

// Start 启动所有代理服务
func (m *Manager) Start(ctx context.Context) error {
	// 启动 HTTP 代理
	m.mu.RLock()
	for port, proxy := range m.proxies {
		if err := m.startProxy(ctx, proxy); err != nil {
			m.mu.RUnlock()
			return fmt.Errorf("启动 HTTP 代理服务失败 [%s:%d]: %w", proxy.config.Label, port, err)
		}
	}
	m.mu.RUnlock()

	// 启动 TCP 代理（使用独立的锁，不阻塞 HTTP）
	for _, proxyCfg := range m.cfg.Proxies {
		if proxyCfg.Type == "tcp" || proxyCfg.Type == "" {
			if proxyCfg.Type == "" {
				proxyCfg.Type = "tcp"
			}
			if err := m.startTCPProxy(proxyCfg); err != nil {
				return fmt.Errorf("启动 TCP 代理服务失败 [%s:%d]: %w", proxyCfg.Label, proxyCfg.Port, err)
			}
		}
	}

	return nil
}

// startProxy 启动单个代理服务
func (m *Manager) startProxy(ctx context.Context, proxy *ProxyService) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", proxy.config.Port))
	if err != nil {
		return err
	}

	proxy.status.StartTime = time.Now()
	proxy.status.Running = true

	proxy.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", proxy.config.Port),
		Handler: proxy,
	}

	go func() {
		logger.L().Info("代理服务已启动",
			zap.Int("port", proxy.config.Port),
			zap.String("label", proxy.config.Label),
		)
		if err := proxy.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.L().Error("代理服务异常退出",
				zap.Int("port", proxy.config.Port),
				zap.String("label", proxy.config.Label),
				zap.Error(err),
			)
			proxy.status.Running = false
		}
	}()

	return nil
}

// Stop 停止所有代理服务
func (m *Manager) Stop() error {
	var errs []error

	// 停止 HTTP 代理
	m.mu.RLock()
	for port, proxy := range m.proxies {
		if proxy.server != nil {
			if err := proxy.server.Shutdown(context.Background()); err != nil {
				errs = append(errs, fmt.Errorf("停止 HTTP 代理服务失败 [%d]: %w", port, err))
			}
			proxy.status.Running = false
		}
	}
	m.mu.RUnlock()

	// 停止 TCP 代理（使用独立的锁）
	m.tcpMu.Lock()
	for port, tcpProxy := range m.tcpProxies {
		if err := tcpProxy.stop(); err != nil {
			errs = append(errs, fmt.Errorf("停止 TCP 代理服务失败 [%d]: %w", port, err))
		}
	}
	m.tcpMu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}

	return nil
}

// GetStatus 获取所有代理状态
func (m *Manager) GetStatus() []interface{} {
	statuses := make([]interface{}, 0, len(m.proxies)+len(m.tcpProxies))

	// HTTP 代理状态
	m.mu.RLock()
	for _, proxy := range m.proxies {
		proxy.mu.RLock()
		status := &ProxyStatus{
			Port:        proxy.status.Port,
			Label:       proxy.status.Label,
			Type:        proxy.status.Type,
			Target:      proxy.status.Target,
			Running:     proxy.status.Running,
			TotalReqs:   atomic.LoadInt64(&proxy.status.TotalReqs),
			SuccessReqs: atomic.LoadInt64(&proxy.status.SuccessReqs),
			FailedReqs:  atomic.LoadInt64(&proxy.status.FailedReqs),
			AvgLatency:  atomic.LoadInt64(&proxy.status.AvgLatency),
			StartTime:   proxy.status.StartTime,
		}

		// 复制最近日志
		status.RecentLogs = make([]RequestLog, len(proxy.status.RecentLogs))
		copy(status.RecentLogs, proxy.status.RecentLogs)
		proxy.mu.RUnlock()

		statuses = append(statuses, status)
	}
	m.mu.RUnlock()

	// TCP 代理状态（使用独立的锁）
	m.tcpMu.Lock()
	for _, tcpProxy := range m.tcpProxies {
		tcpProxy.mu.RLock()
		status := &TCPProxyStatus{
			Port:          tcpProxy.status.Port,
			Label:         tcpProxy.status.Label,
			Type:          tcpProxy.status.Type,
			Target:        tcpProxy.status.Target,
			Running:       tcpProxy.status.Running,
			TotalConns:    atomic.LoadInt64(&tcpProxy.status.TotalConns),
			ActiveConns:   atomic.LoadInt64(&tcpProxy.status.ActiveConns),
			UploadBytes:   atomic.LoadInt64(&tcpProxy.status.UploadBytes),
			DownloadBytes: atomic.LoadInt64(&tcpProxy.status.DownloadBytes),
			StartTime:     tcpProxy.status.StartTime,
		}

		// 复制最近日志
		status.RecentLogs = make([]TCPLog, len(tcpProxy.status.RecentLogs))
		copy(status.RecentLogs, tcpProxy.status.RecentLogs)
		tcpProxy.mu.RUnlock()

		statuses = append(statuses, status)
	}
	m.tcpMu.Unlock()

	return statuses
}

// GetProxy 获取指定端口的代理服务
func (m *Manager) GetProxy(port int) *ProxyService {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.proxies[port]
}

// GetTCPProxy 获取指定端口的 TCP 代理服务
func (m *Manager) GetTCPProxy(port int) *TCPProxyService {
	m.tcpMu.Lock()
	defer m.tcpMu.Unlock()
	return m.tcpProxies[port]
}

// ServeHTTP 实现 http.Handler 接口
func (p *ProxyService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// 记录请求数据
	var requestData string
	if r.Method != "GET" && r.ContentLength > 0 && r.ContentLength < 1024*10 {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		requestData = string(body)
		r.Body = io.NopCloser(strings.NewReader(requestData))
	}

	// 创建响应记录器
	recorder := &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// 执行代理
	p.reverseProxy.ServeHTTP(recorder, r)

	// 计算延迟
	latency := time.Since(startTime).Milliseconds()

	// 记录响应数据
	var responseData string
	if recorder.responseData != nil && len(recorder.responseData) < 1024*10 {
		responseData = string(recorder.responseData)
	}

	// 更新统计
	atomic.AddInt64(&p.status.TotalReqs, 1)
	if recorder.statusCode >= 200 && recorder.statusCode < 400 {
		atomic.AddInt64(&p.status.SuccessReqs, 1)
	} else {
		atomic.AddInt64(&p.status.FailedReqs, 1)
	}

	// 更新平均延迟
	totalLatency := atomic.AddInt64(&p.status.TotalLatency, latency)
	totalReqs := atomic.LoadInt64(&p.status.TotalReqs)
	avgLatency := totalLatency / totalReqs
	atomic.StoreInt64(&p.status.AvgLatency, avgLatency)

	// 记录日志
	logEntry := RequestLog{
		Timestamp:    startTime,
		Method:       r.Method,
		Path:         r.URL.Path,
		StatusCode:   recorder.statusCode,
		Latency:      latency,
		RequestData:  requestData,
		ResponseData: responseData,
	}

	p.mu.Lock()
	p.status.RecentLogs = append(p.status.RecentLogs, logEntry)
	// 保留最近100条
	if len(p.status.RecentLogs) > 100 {
		p.status.RecentLogs = p.status.RecentLogs[1:]
	}
	p.mu.Unlock()

	// 记录到日志
	logger.L().Debug("请求完成",
		zap.Int("port", p.config.Port),
		zap.String("label", p.config.Label),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.Int("status", recorder.statusCode),
		zap.Int64("latency", latency),
	)
}

// responseRecorder 响应记录器
type responseRecorder struct {
	http.ResponseWriter
	statusCode   int
	responseData []byte
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	r.responseData = append(r.responseData, data...)
	return r.ResponseWriter.Write(data)
}
