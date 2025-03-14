package logger

import (
	"os"
	"sync"

	"github.com/mini-maxit/worker/internal/constants"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	timeKey   = "time"
	levelKey  = "level"
	sourceKey = "source"
	msgKey    = "msg"
)

var (
	sugarLogger  *zap.SugaredLogger
	currentLevel zap.AtomicLevel
	once         sync.Once
)

func InitializeLogger() {
	once.Do(func() {
		currentLevel = zap.NewAtomicLevelAt(zap.InfoLevel) // Default to InfoLevel

		fileSync := zapcore.AddSync(&lumberjack.Logger{
			Filename: constants.DefaultLogPath,
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
			currentLevel, // Use dynamic log level
		)

		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			consoleSync,
			currentLevel, // Use dynamic log level
		)

		core := zapcore.NewTee(fileCore, consoleCore)

		logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
		sugarLogger = logger.Sugar()
	})
}

func NewNamedLogger(serviceName string) *zap.SugaredLogger {
	if sugarLogger == nil {
		InitializeLogger()
	}
	return sugarLogger.Named(serviceName)
}

func SetLoggerLevel(level zapcore.Level) {
	if sugarLogger == nil {
		InitializeLogger()
	}
	currentLevel.SetLevel(level)
}
