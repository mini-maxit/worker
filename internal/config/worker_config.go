package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/mini-maxit/worker/logger"
)

const (
	DefailtRabbitmqHost     = "localhost"
	DefaultRabbitmqUser     = "guest"
	DefaultRabbitmqPassword = "guest"
	DefaultRabbitmqPort     = "5672"
)

type Config struct {
	RQUrl string
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
		logger.Warnf("RABBITMQ_HOST is not set, using default value %s", DefailtRabbitmqHost)
	}
	rabbitmqPortStr := os.Getenv("RABBITMQ_PORT")
	if rabbitmqPortStr == "" {
		logger.Warnf("RABBITMQ_PORT is not set, using default value %s", DefaultRabbitmqPort)
	}
	rabbitmqPort, err := strconv.ParseUint(rabbitmqPortStr, 10, 16)
	if err != nil {
		logger.Fatalf("failed to parse RABBITMQ_PORT with error: %v", err)
	}
	rabbitmqUser := os.Getenv("RABBITMQ_USER")
	if rabbitmqUser == "" {
		logger.Warnf("RABBITMQ_USER is not set, using default value %s", DefaultRabbitmqUser)
	}
	rabbitmqPassword := os.Getenv("RABBITMQ_PASSWORD")
	if rabbitmqPassword == "" {
		logger.Warnf("RABBITMQ_PASSWORD is not set, using default value %s", DefaultRabbitmqPassword)
	}

	rabbitmqUrl := fmt.Sprintf("amqp://%s:%s@%s:%d/", rabbitmqUser, rabbitmqPassword, rabbitmqHost, rabbitmqPort)

	return &Config{
		RQUrl: rabbitmqUrl,
	}
}
