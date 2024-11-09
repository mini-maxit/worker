package logger

import (
	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"sort"
)

const logPath = "./logger/logs/log.txt"

const (
    timeKey   = "time"
    levelKey  = "level"
    sourceKey = "source"
    msgKey    = "msg"
)

func customSort(keys []string) {
    order := map[string]int{
        timeKey:   0,
        levelKey:  1,
        sourceKey: 2,
        msgKey:    3,
    }
    sort.Slice(keys, func(i, j int) bool {
        return order[keys[i]] < order[keys[j]]
    })
}

var lumberjackLogger = &lumberjack.Logger{
	Filename:   logPath,
	MaxAge:     1,
	Compress:   true,

}

var logFormatter = &log.TextFormatter{
	FullTimestamp: true,
	DisableQuote:  true,
	SortingFunc:  customSort,
}

func InitializeLogger() {
	log.SetFormatter(logFormatter)
	log.SetOutput(lumberjackLogger)
}

func NewNamedLogger(serviceName string) *log.Entry {
	contextLogger := log.WithFields(log.Fields{
		"source": serviceName,
	})
	return contextLogger
}
