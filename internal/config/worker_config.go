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
	RabbitMQUrl    string
	FileStorageUrl string
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

	rabbitmqHost := os.Getenv("RABBITMQ_HOST")
	if rabbitmqHost == "" {
		rabbitmqHost = constants.DefailtRabbitmqHost
		logger.Warnf("RABBITMQ_HOST is not set, using default value %s", constants.DefailtRabbitmqHost)
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

	rabbitmqUrl := fmt.Sprintf("amqp://%s:%s@%s:%d/", rabbitmqUser, rabbitmqPassword, rabbitmqHost, rabbitmqPort)
	fileStorageUrl := fmt.Sprintf("http://%s:%d", fileStorageHost, fileStoragePort)

	return &Config{
		RabbitMQUrl:    rabbitmqUrl,
		FileStorageUrl: fileStorageUrl,
	}
}
