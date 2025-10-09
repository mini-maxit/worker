package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/pkg/constants"
)

type Config struct {
	RabbitMQURL       string
	StorageBaseUrl    string
	ConsumeQueueName  string
	ResponseQueueName string
	MaxWorkers        int
	JobsDataVolume    string
	VerifierFlags     []string
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
	storageBaseUrl := storageConfig()
	workerQueueName, responseQueueName, maxWorkers := workerConfig()
	jobsDataVolume := dockerConfig()
	verifierFlagsStr := verifierConfig()
	
	return &Config{
		RabbitMQURL:       rabbitmqURL,
		StorageBaseUrl:    storageBaseUrl,
		ConsumeQueueName:  workerQueueName,
		ResponseQueueName: responseQueueName,
		MaxWorkers:        int(maxWorkers),
		JobsDataVolume:    jobsDataVolume,
		VerifierFlags:     verifierFlagsStr,
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

func storageConfig() string {
	logger := logger.NewNamedLogger("config")

	storageHost := os.Getenv("STORAGE_HOST")
	if storageHost == "" {
		storageHost = constants.DefaultStorageHost
		logger.Warnf("STORAGE_HOST is not set, using default value %s", constants.DefaultStorageHost)
	}
	storagePortStr := os.Getenv("STORAGE_PORT")
	if storagePortStr == "" {
		storagePortStr = constants.DefaultStoragePort
		logger.Warnf("STORAGE_PORT is not set, using default value %s", constants.DefaultStoragePort)
	}
	storagePort, err := strconv.ParseUint(storagePortStr, 10, 16)
	if err != nil {
		logger.Fatalf("failed to parse STORAGE_PORT with error: %v", err)
	}

	storageURL := fmt.Sprintf("http://%s:%d", storageHost, storagePort)

	return storageURL
}

func workerConfig() (string, string, int64) {
	logger := logger.NewNamedLogger("config")

	workerQueueName := os.Getenv("WORKER_QUEUE_NAME")
	if workerQueueName == "" {
		workerQueueName = constants.DefaultWorkerQueueName
		logger.Warnf("WORKER_QUEUE_NAME is not set, using default value %s", constants.DefaultWorkerQueueName)
	}
	responseQueueName := os.Getenv("RESPONSE_QUEUE_NAME")
	if responseQueueName == "" {
		responseQueueName = constants.DefaultResponseQueueName
		logger.Warnf("RESPONSE_QUEUE_NAME is not set, using default value %s", constants.DefaultResponseQueueName)
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

	return workerQueueName, responseQueueName, maxWorkers
}

func dockerConfig() string {
	logger := logger.NewNamedLogger("config")

	jobsDataVolume := os.Getenv("JOBS_DATA_VOLUME")
	if jobsDataVolume == "" {
		jobsDataVolume = constants.DefaultJobsDataVolume
		logger.Warnf("JOBS_DATA_VOLUME is not set, using default value %s", constants.DefaultJobsDataVolume)
	}

	return jobsDataVolume
}

func verifierConfig() []string {
	logger := logger.NewNamedLogger("config")
	
	verifierFlagsStr := os.Getenv("VERIFIER_FLAGS")
	if verifierFlagsStr == "" {
		verifierFlagsStr = constants.DefaultVerifierFlags
		logger.Warnf("VERIFIER_FLAGS is not set, using default value %s", constants.DefaultVerifierFlags)
	}

	return strings.Split(verifierFlagsStr, ",")
}