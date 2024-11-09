package main

import (
	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/worker"
	"github.com/mini-maxit/worker/logger"

)

func main() {
		// Initialize the logger
		logger.InitializeLogger()

		// Load the configuration
		config := config.LoadWorkerConfig()

		// Connect to RabbitMQ
		conn := worker.NewRabbitMqConnection(config)

		defer conn.Close()

		// Start the worker
		worker.Work(conn)
}
