package main

import (
	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/worker"
)

func main() {
		// Load the configuration
		config := config.LoadWorkerConfig()

		// Connect to RabbitMQ
		_, ch := worker.NewRabbitMQ(config)

		// Start the worker
		worker.Work(ch)
}
