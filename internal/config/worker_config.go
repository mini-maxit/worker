package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	RQUrl string
}

func LoadWorkerConfig() Config {

	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal(err)
	}

	rabbitmqHost := os.Getenv("RABBITMQ_HOST")
	rabbitmqUser := os.Getenv("RABBITMQ_USER")
	rabbitmqPassword := os.Getenv("RABBITMQ_PASSWORD")
	rabbitmqPort := os.Getenv("RABBITMQ_PORT")


	rabbitmqUrl := "amqp://" + rabbitmqUser + ":" + rabbitmqPassword + "@" + rabbitmqHost + ":" + rabbitmqPort + "/"

	return Config{
		RQUrl: rabbitmqUrl,
	}
}
