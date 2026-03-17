package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config 配置结构
type Config struct {
	Proxies []ProxyConfig `toml:"proxies"`
	Log     LogConfig     `toml:"log"`
	Monitor MonitorConfig `toml:"monitor"`
}

// ProxyConfig 代理服务配置
type ProxyConfig struct {
	Port   int    `toml:"port"`
	Label  string `toml:"label"`
	Target string `toml:"target"`
}

// LogConfig 日志配置
type LogConfig struct {
	Dir        string `toml:"dir"`
	Level      string `toml:"level"`
	MaxSize    int    `toml:"maxSize"`    // MB
	MaxBackups int    `toml:"maxBackups"` // 保留文件数
}

// MonitorConfig 监控配置
type MonitorConfig struct {
	Enabled bool `toml:"enabled"`
	Port    int  `toml:"port"`
}

// Load 加载配置文件
func Load(filepath string) (*Config, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	if cfg.Log.Dir == "" {
		cfg.Log.Dir = "./logs"
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.MaxSize == 0 {
		cfg.Log.MaxSize = 100
	}
	if cfg.Log.MaxBackups == 0 {
		cfg.Log.MaxBackups = 100
	}
	if cfg.Monitor.Port == 0 {
		cfg.Monitor.Port = 9090
	}

	return &cfg, nil
}
