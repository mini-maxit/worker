package main

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/mini-maxit/worker/worker"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal(err)
	}

	worker.Work()
}
