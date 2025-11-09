package logger

import (
	"os"
	"path/filepath"
	"runtime"

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

var sugarLogger *zap.SugaredLogger

// getProjectRoot finds the project root directory by looking for go.mod file.
func getProjectRoot() string {
	// Get the current file's directory.
	_, currentFile, _, ok := runtime.Caller(0)
	var currentDir string
	if !ok || currentFile == "" {
		wd, _ := os.Getwd()
		currentDir = wd
	} else {
		currentDir = filepath.Dir(currentFile)
	}

	// Walk up the directory tree to find go.mod.
	for {
		goModPath := filepath.Join(currentDir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return currentDir
		}

		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			// Reached the root directory, fallback to current working directory.
			wd, _ := os.Getwd()
			return wd
		}
		currentDir = parent
	}
}

// getLogPath returns the absolute path to the log file in the project root.
func getLogPath() string {
	projectRoot := getProjectRoot()
	logDir := os.Getenv("LOG_DIR")
	if logDir == "" {
		logDir = "logs"
	}

	return filepath.Join(projectRoot, logDir, "app.log")
}

func initializeLogger() {
	logPath := getLogPath()

	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		logPath = "app.log"
	}

	w := zapcore.AddSync(&lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    50,
		MaxBackups: 10,
		MaxAge:     28,
		Compress:   true,
		LocalTime:  true,
	})

	stdWriter := zapcore.AddSync(os.Stdout)

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        timeKey,
		LevelKey:       levelKey,
		NameKey:        sourceKey,
		MessageKey:     msgKey,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}

	fileCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		w,
		zap.InfoLevel,
	)

	stdCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		stdWriter,
		zap.InfoLevel,
	)

	core := zapcore.NewTee(fileCore, stdCore)

	log := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	sugarLogger = log.Sugar()
}

// NewNamedLogger creates a new named SugaredLogger for a given service.
func NewNamedLogger(name string) *zap.SugaredLogger {
	if sugarLogger == nil {
		initializeLogger()
	}
	return sugarLogger.Named(name)
}
