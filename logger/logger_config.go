package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const logPath = "./logger/logs/log.txt"

const (
	timeKey   = "time"
	levelKey  = "level"
	sourceKey = "source"
	msgKey    = "msg"
)

var sugarLogger *zap.SugaredLogger

// InitializeLogger sets up Zap with a custom configuration and initializes the SugaredLogger
func InitializeLogger() {
	// Configure log rotation with lumberjack
	w := zapcore.AddSync(&lumberjack.Logger{
		Filename: logPath,
		MaxAge:   1,
		Compress: true,
	})

	// Encoder configuration for Console format
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        timeKey,
		LevelKey:       levelKey,
		NameKey:        "source",
		MessageKey:     msgKey,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}

	// Create the core
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		w,
		zap.InfoLevel,
	)

	// Initialize the sugared logger
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	sugarLogger = logger.Sugar()
}

// NewNamedLogger creates a new named SugaredLogger for a given service
func NewNamedLogger(serviceName string) *zap.SugaredLogger {
	if sugarLogger == nil {
		InitializeLogger()
	}
	return sugarLogger.Named(serviceName)
}
