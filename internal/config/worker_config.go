package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/logger"
)

type Config struct {
	RabbitMQURL     string
	FileStorageURL  string
	WorkerQueueName string
	MaxWorkers      int
}

func NewConfig() *Config {
	logger := logger.NewNamedLogger("config")

	_, err := os.Stat(".env")
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Fatalf("failed to stat .env file with error: %v", err)
		}
	} else {
		if os.Getenv("ENV") != "PROD" {
			logger.Warn(".env file detected in production environment. This is not recommended.")
		}
		err = godotenv.Load(".env")
		if err != nil {
			logger.Fatalf("failed to load .env file with error: %v", err)
		}
	}

	rabbitmqURL := rabbitmqConfig()
	fileStorageURL := fileStorageConfig()
	workerQueueName, maxWorkers := workerConfig()

	return &Config{
		RabbitMQURL:     rabbitmqURL,
		FileStorageURL:  fileStorageURL,
		WorkerQueueName: workerQueueName,
		MaxWorkers:      int(maxWorkers),
	}
}

func rabbitmqConfig() string {
	logger := logger.NewNamedLogger("config")

	rabbitmqHost := os.Getenv("RABBITMQ_HOST")
	if rabbitmqHost == "" {
		rabbitmqHost = constants.DefaultRabbitmqHost
		logger.Warnf("RABBITMQ_HOST is not set, using default value %s", constants.DefaultRabbitmqHost)
	}
	rabbitmqPortStr := os.Getenv("RABBITMQ_PORT")
	if rabbitmqPortStr == "" {
		rabbitmqPortStr = constants.DefaultRabbitmqPort
		logger.Warnf("RABBITMQ_PORT is not set, using default value %s", constants.DefaultRabbitmqPort)
	}
	rabbitmqPort, err := strconv.ParseUint(rabbitmqPortStr, 10, 16)
	if err != nil {
		logger.Fatalf("failed to parse RABBITMQ_PORT with error: %v", err)
	}
	rabbitmqUser := os.Getenv("RABBITMQ_USER")
	if rabbitmqUser == "" {
		rabbitmqUser = constants.DefaultRabbitmqUser
		logger.Warnf("RABBITMQ_USER is not set, using default value %s", constants.DefaultRabbitmqUser)
	}
	rabbitmqPassword := os.Getenv("RABBITMQ_PASSWORD")
	if rabbitmqPassword == "" {
		rabbitmqPassword = constants.DefaultRabbitmqPassword
		logger.Warnf("RABBITMQ_PASSWORD is not set, using default value %s", constants.DefaultRabbitmqPassword)
	}

	rabbitmqURL := fmt.Sprintf("amqp://%s:%s@%s:%d/", rabbitmqUser, rabbitmqPassword, rabbitmqHost, rabbitmqPort)

	return rabbitmqURL
}

func fileStorageConfig() string {
	logger := logger.NewNamedLogger("config")

	fileStorageHost := os.Getenv("FILESTORAGE_HOST")
	if fileStorageHost == "" {
		fileStorageHost = constants.DefaultFileStorageHost
		logger.Warnf("FILESTORAGE_HOST is not set, using default value %s", constants.DefaultFileStorageHost)
	}
	fileStoragePortStr := os.Getenv("FILESTORAGE_PORT")
	if fileStoragePortStr == "" {
		fileStoragePortStr = constants.DefaultFilesStoragePort
		logger.Warnf("FILESTORAGE_PORT is not set, using default value %s", constants.DefaultFilesStoragePort)
	}
	fileStoragePort, err := strconv.ParseUint(fileStoragePortStr, 10, 16)
	if err != nil {
		logger.Fatalf("failed to parse FILESTORAGE_PORT with error: %v", err)
	}

	fileStorageURL := fmt.Sprintf("http://%s:%d", fileStorageHost, fileStoragePort)

	return fileStorageURL
}

func workerConfig() (string, int64) {
	logger := logger.NewNamedLogger("config")

	workerQueueName := os.Getenv("WORKER_QUEUE_NAME")
	if workerQueueName == "" {
		workerQueueName = constants.DefaultWorkerQueueName
		logger.Warnf("WORKER_QUEUE_NAME is not set, using default value %s", constants.DefaultWorkerQueueName)
	}
	maxWorkersStr := os.Getenv("MAX_WORKERS")
	if maxWorkersStr == "" {
		maxWorkersStr = constants.DefaultMaxWorkersStr
		logger.Warnf("MAX_WORKERS is not set, using default value %s", constants.DefaultMaxWorkersStr)
	}
	maxWorkers, err := strconv.ParseInt(maxWorkersStr, 10, 8)
	if err != nil {
		logger.Fatalf("failed to parse MAX_WORKERS with error: %v", err)
	}

	return workerQueueName, maxWorkers
}
