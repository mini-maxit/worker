package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBUser	 string
	DBPassword string
	DBName string
	DBSslMode string
	RQUrl string
}

func LoadWorkerConfig() Config {
	
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal(err)
	}

	return Config{
		DBUser: os.Getenv("DATABASE_USER"),
		DBPassword: os.Getenv("DATABASE_PASSWORD"),
		DBName: os.Getenv("DATABASE_NAME"),
		DBSslMode: os.Getenv("DATABASE_SSL_MODE"),
		RQUrl: os.Getenv("RABBITMQ_URL"),
	}
}
