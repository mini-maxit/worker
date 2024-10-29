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

	return Config{
		RQUrl: os.Getenv("RABBITMQ_URL"),
	}
}
