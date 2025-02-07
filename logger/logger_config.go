package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
)

const logPath = "./logger/logs/worker/log.txt"

const (
	timeKey   = "time"
	levelKey  = "level"
	sourceKey = "source"
	msgKey    = "msg"
)

var sugarLogger *zap.SugaredLogger

func InitializeLogger() {
	fileSync := zapcore.AddSync(&lumberjack.Logger{
		Filename: logPath,
		MaxAge:   1,
		Compress: true,
	})

	consoleSync := zapcore.AddSync(os.Stdout)

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        timeKey,
		LevelKey:       levelKey,
		NameKey:        "source",
		MessageKey:     msgKey,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}

	fileCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		fileSync,
		zap.InfoLevel,
	)

	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		consoleSync,
		zap.InfoLevel,
	)

	core := zapcore.NewTee(fileCore, consoleCore)

	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	sugarLogger = logger.Sugar()
}

func NewNamedLogger(serviceName string) *zap.SugaredLogger {
	if sugarLogger == nil {
		InitializeLogger()
	}
	return sugarLogger.Named(serviceName)
}
