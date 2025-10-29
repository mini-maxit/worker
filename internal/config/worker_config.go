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
	RabbitMQURL      string
	PublishChanSize  int
	StorageBaseUrl   string
	ConsumeQueueName string
	MaxWorkers       int
	JobsDataVolume   string
	VerifierFlags    []string
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

	rabbitmqURL, publishChanSize := rabbitmqConfig()
	storageBaseUrl := storageConfig()
	workerQueueName, maxWorkers := workerConfig()
	jobsDataVolume := dockerConfig()
	verifierFlagsStr := verifierConfig()

	return &Config{
		RabbitMQURL:      rabbitmqURL,
		PublishChanSize:  publishChanSize,
		StorageBaseUrl:   storageBaseUrl,
		ConsumeQueueName: workerQueueName,
		MaxWorkers:       int(maxWorkers),
		JobsDataVolume:   jobsDataVolume,
		VerifierFlags:    verifierFlagsStr,
	}
}

func rabbitmqConfig() (string, int) {
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
	var publishChanSize int
	publishChanSizeStr := os.Getenv("RABBITMQ_PUBLISH_CHAN_SIZE")
	if publishChanSizeStr == "" {
		publishChanSize = constants.DefaultRabbitmqPublishChanSize
		logger.Warnf("RABBITMQ_PUBLISH_CHAN_SIZE is not set, using default value %d",
			constants.DefaultRabbitmqPublishChanSize)
	} else {
		publishChanSize, err = strconv.Atoi(publishChanSizeStr)
		if err != nil {
			logger.Fatalf("failed to parse RABBITMQ_PUBLISH_CHAN_SIZE with error: %v", err)
		}
	}

	rabbitmqURL := fmt.Sprintf("amqp://%s:%s@%s:%d/", rabbitmqUser, rabbitmqPassword, rabbitmqHost, rabbitmqPort)

	return rabbitmqURL, int(publishChanSize)
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

func workerConfig() (string, int64) {
	logger := logger.NewNamedLogger("config")

	workerQueueName := os.Getenv("WORKER_QUEUE_NAME")
	if workerQueueName == "" {
		workerQueueName = constants.DefaultWorkerQueueName
		logger.Warnf("WORKER_QUEUE_NAME is not set, using default value %s", constants.DefaultWorkerQueueName)
	}
	var maxWorkers int64
	var err error
	maxWorkersStr := os.Getenv("MAX_WORKERS")
	if maxWorkersStr == "" {
		maxWorkers = constants.DefaultMaxWorkers
		logger.Warnf("MAX_WORKERS is not set, using default value %d", constants.DefaultMaxWorkers)
	} else {
		maxWorkers, err = strconv.ParseInt(maxWorkersStr, 10, 8)
		if err != nil {
			logger.Fatalf("failed to parse MAX_WORKERS with error: %v", err)
		}
	}

	return workerQueueName, maxWorkers
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
