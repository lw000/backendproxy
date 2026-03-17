package logger

import (
	"os"

	"backendproxy/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	log *zap.Logger
	sugaredLog *zap.SugaredLogger
)

// Init 初始化日志
func Init(cfg config.LogConfig) error {
	// 确保日志目录存在
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return err
	}

	// 日志文件切割配置
	hook := lumberjack.Logger{
		Filename:   cfg.Dir + "/proxy.log",
		MaxSize:    cfg.MaxSize,    // MB
		MaxBackups: cfg.MaxBackups, // 保留文件数
		MaxAge:     0,              // 不按天数删除
		Compress:   false,          // 不压缩
	}

	// 解析日志级别
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	// 编码器配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 创建 Core
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(&hook),
		level,
	)

	// 创建 Logger
	log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	sugaredLog = log.Sugar()

	return nil
}

// L 获取 Logger
func L() *zap.Logger {
	if log == nil {
		return zap.NewNop()
	}
	return log
}

// S 获取 SugaredLogger
func S() *zap.SugaredLogger {
	if sugaredLog == nil {
		return zap.NewNop().Sugar()
	}
	return sugaredLog
}

// Sync 同步日志
func Sync() {
	if log != nil {
		_ = log.Sync()
	}
}
