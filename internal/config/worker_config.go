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

	rabbitmq_host := os.Getenv("RABBITMQ_HOST")
	rabbitmq_user := os.Getenv("USER")
	rabbitmq_password := os.Getenv("PASSWORD")
	rabbitmq_port := os.Getenv("RABBITMQ_PORT")


	rabbitmq_url := "amqp://" + rabbitmq_user + ":" + rabbitmq_password + "@" + rabbitmq_host + ":" + rabbitmq_port + "/"

	return Config{
		RQUrl: rabbitmq_url,
	}
}
