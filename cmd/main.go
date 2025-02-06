package main

import (
	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/logger"
	"github.com/mini-maxit/worker/worker"
)

func main() {
	// Initialize the logger
	logger.InitializeLogger()

	// Load the configuration
	config := config.NewConfig()

	// Connect to RabbitMQ
	conn := worker.NewRabbitMqConnection(config)

	defer conn.Close()

	// Start the worker
	worker := worker.NewWorker(conn)
	worker.Work()
}
