package proxy

import (
	"backendproxy/config"
	"backendproxy/logger"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// TCPLog TCP 连接日志
type TCPLog struct {
	Timestamp     time.Time `json:"timestamp"`
	ClientAddr    string    `json:"client_addr"`
	Duration      int64     `json:"duration"`      // 毫秒
	UploadBytes   int64     `json:"upload_bytes"`
	DownloadBytes int64     `json:"download_bytes"`
	UploadHex     string    `json:"upload_hex"`
	DownloadHex   string    `json:"download_hex"`
}

// TCPProxyStatus TCP 代理状态
type TCPProxyStatus struct {
	Port          int        `json:"port"`
	Label         string     `json:"label"`
	Type          string     `json:"type"`
	Target        string     `json:"target"`
	Running       bool       `json:"running"`
	TotalConns    int64      `json:"total_conns"`
	ActiveConns   int64      `json:"active_conns"`
	UploadBytes   int64      `json:"upload_bytes"`
	DownloadBytes int64      `json:"download_bytes"`
	RecentLogs    []TCPLog   `json:"recent_logs"`
	StartTime     time.Time  `json:"start_time"`
}

// TCPProxyService TCP 代理服务
type TCPProxyService struct {
	config      config.ProxyConfig
	status      *TCPProxyStatus
	mu          sync.RWMutex
	listener    net.Listener
	connWg      sync.WaitGroup
	connMu      sync.Mutex
	connections map[net.Conn]struct{}
}

// startTCPProxy 启动 TCP 代理
func (m *Manager) startTCPProxy(cfg config.ProxyConfig) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return err
	}

	service := &TCPProxyService{
		config: cfg,
		status: &TCPProxyStatus{
			Port:        cfg.Port,
			Label:       cfg.Label,
			Type:        "tcp",
			Target:      cfg.Target,
			Running:     true,
			RecentLogs:  make([]TCPLog, 0, 100),
			StartTime:   time.Now(),
		},
		listener:    listener,
		connections: make(map[net.Conn]struct{}),
	}

	// 使用 TCP 专用锁，避免与 HTTP 读写锁冲突
	m.tcpMu.Lock()
	m.tcpProxies[cfg.Port] = service
	m.tcpMu.Unlock()

	go service.serve()

	logger.L().Info("TCP 代理服务已启动",
		zap.Int("port", cfg.Port),
		zap.String("label", cfg.Label),
		zap.String("target", cfg.Target),
	)

	return nil
}

// serve 启动 TCP 代理服务
func (s *TCPProxyService) serve() {
	for {
		clientConn, err := s.listener.Accept()
		if err != nil {
			if s.status.Running {
				logger.L().Error("TCP 代理接受连接失败",
					zap.Int("port", s.config.Port),
					zap.String("label", s.config.Label),
					zap.Error(err),
				)
			}
			return
		}

		s.connWg.Add(1)
		go s.handleConnection(clientConn)
	}
}

// handleConnection 处理 TCP 连接
func (s *TCPProxyService) handleConnection(clientConn net.Conn) {
	defer s.connWg.Done()
	defer clientConn.Close()

	// 注册连接
	s.connMu.Lock()
	s.connections[clientConn] = struct{}{}
	s.connMu.Unlock()
	defer func() {
		s.connMu.Lock()
		delete(s.connections, clientConn)
		s.connMu.Unlock()
	}()

	atomic.AddInt64(&s.status.TotalConns, 1)
	atomic.AddInt64(&s.status.ActiveConns, 1)
	defer atomic.AddInt64(&s.status.ActiveConns, -1)

	startTime := time.Now()
	clientAddr := clientConn.RemoteAddr().String()

	// 连接目标服务
	targetConn, err := net.Dial("tcp", s.config.Target)
	if err != nil {
		logger.L().Error("TCP 代理连接目标失败",
			zap.Int("port", s.config.Port),
			zap.String("label", s.config.Label),
			zap.String("client", clientAddr),
			zap.String("target", s.config.Target),
			zap.Error(err),
		)
		return
	}
	defer targetConn.Close()

	// 启用 TCP Keep-Alive，保持长连接
	if tcpConn, ok := clientConn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}
	if tcpConn, ok := targetConn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	// 设置读写超时，防止无限阻塞
	deadline := time.Now().Add(300 * time.Second) // 5 分钟无数据传输则超时
	clientConn.SetReadDeadline(deadline)
	targetConn.SetReadDeadline(deadline)

	// 注册目标连接
	s.connMu.Lock()
	s.connections[targetConn] = struct{}{}
	s.connMu.Unlock()
	defer func() {
		s.connMu.Lock()
		delete(s.connections, targetConn)
		s.connMu.Unlock()
	}()

	// 创建双向数据转发
	var uploadBytes, downloadBytes int64
	var uploadData, downloadData []byte

	// 使用更大的缓冲区提高吞吐量（64KB 适合大多数数据库和缓存场景）
	uploadBuffer := make([]byte, 64*1024)
	downloadBuffer := make([]byte, 64*1024)

	// 客户端到服务器的数据转发
	uploadDone := make(chan struct{})
	go func() {
		defer close(uploadDone)
		for {
			n, err := clientConn.Read(uploadBuffer)
			if err != nil {
				if err != io.EOF {
					logger.L().Debug("客户端读错误",
						zap.Int("port", s.config.Port),
						zap.String("client", clientAddr),
						zap.Error(err),
					)
				}
				return
			}

			atomic.AddInt64(&s.status.UploadBytes, int64(n))
			atomic.AddInt64(&uploadBytes, int64(n))

			// 记录上传数据（最多 1KB）
			if len(uploadData) < 1024 {
				end := len(uploadData) + n
				if end > 1024 {
					end = 1024
				}
				uploadData = append(uploadData, uploadBuffer[:n]...)
			}

			if _, err := targetConn.Write(uploadBuffer[:n]); err != nil {
				logger.L().Debug("目标服务器写错误",
					zap.Int("port", s.config.Port),
					zap.String("target", s.config.Target),
					zap.Error(err),
				)
				return
			}
		}
	}()

	// 服务器到客户端的数据转发
	downloadDone := make(chan struct{})
	go func() {
		defer close(downloadDone)
		for {
			n, err := targetConn.Read(downloadBuffer)
			if err != nil {
				if err != io.EOF {
					logger.L().Debug("目标服务器读错误",
						zap.Int("port", s.config.Port),
						zap.String("target", s.config.Target),
						zap.Error(err),
					)
				}
				return
			}

			atomic.AddInt64(&s.status.DownloadBytes, int64(n))
			atomic.AddInt64(&downloadBytes, int64(n))

			// 记录下载数据（最多 1KB）
			if len(downloadData) < 1024 {
				end := len(downloadData) + n
				if end > 1024 {
					end = 1024
				}
				downloadData = append(downloadData, downloadBuffer[:n]...)
			}

			if _, err := clientConn.Write(downloadBuffer[:n]); err != nil {
				logger.L().Debug("客户端写错误",
					zap.Int("port", s.config.Port),
					zap.String("client", clientAddr),
					zap.Error(err),
				)
				return
			}
		}
	}()

	// 等待任一方向的转发结束
	select {
	case <-uploadDone:
	case <-downloadDone:
	}

	duration := time.Since(startTime).Milliseconds()

	// 记录日志
	logEntry := TCPLog{
		Timestamp:     startTime,
		ClientAddr:    clientAddr,
		Duration:      duration,
		UploadBytes:   uploadBytes,
		DownloadBytes: downloadBytes,
		UploadHex:     toHexString(uploadData),
		DownloadHex:   toHexString(downloadData),
	}

	s.mu.Lock()
	s.status.RecentLogs = append(s.status.RecentLogs, logEntry)
	if len(s.status.RecentLogs) > 100 {
		s.status.RecentLogs = s.status.RecentLogs[1:]
	}
	s.mu.Unlock()

	logger.L().Info("TCP 连接完成",
		zap.Int("port", s.config.Port),
		zap.String("label", s.config.Label),
		zap.String("client", clientAddr),
		zap.Int64("duration", duration),
		zap.Int64("upload", uploadBytes),
		zap.Int64("download", downloadBytes),
		zap.Int("upload_hex_len", len(uploadData)),
		zap.Int("download_hex_len", len(downloadData)),
	)
}

// stopTCPProxy 停止 TCP 代理
func (s *TCPProxyService) stop() error {
	s.status.Running = false

	// 关闭监听器，停止接受新连接
	if s.listener != nil {
		_ = s.listener.Close()
	}

	// 主动关闭所有活跃连接
	s.connMu.Lock()
	for conn := range s.connections {
		_ = conn.Close()
	}
	s.connMu.Unlock()

	// 等待所有连接完成
	s.connWg.Wait()
	return nil
}

// toHexString 将字节数组转换为十六进制字符串
func toHexString(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	hexStr := hex.EncodeToString(data)
	// 每 64 个字符换行（32 字节）
	var result []string
	for i := 0; i < len(hexStr); i += 64 {
		end := i + 64
		if end > len(hexStr) {
			end = len(hexStr)
		}
		result = append(result, hexStr[i:end])
	}
	return strings.Join(result, "\n")
}

// formatHex 格式化十六进制显示（带偏移量）
func formatHex(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	var result []string
	for i := 0; i < len(data); i += 16 {
		line := fmt.Sprintf("%04x: ", i)

		// 十六进制
		hexPart := make([]string, 16)
		for j := 0; j < 16; j++ {
			if i+j < len(data) {
				hexPart[j] = fmt.Sprintf("%02x", data[i+j])
			} else {
				hexPart[j] = "  "
			}
			if j%4 == 3 && j < 15 {
				hexPart[j] += " "
			}
		}
		line += strings.Join(hexPart, " ")

		// ASCII
		line += " |"
		for j := 0; j < 16 && i+j < len(data); j++ {
			if data[i+j] >= 32 && data[i+j] <= 126 {
				line += string(data[i+j])
			} else {
				line += "."
			}
		}
		line += "|"

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}
