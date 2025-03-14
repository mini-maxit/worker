package main

import (
	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/services"
	"github.com/mini-maxit/worker/rabbitmq"
)

func main() {
	// Initialize the logger
	logger.InitializeLogger()

	// Load the configuration
	config := config.NewConfig()

	// Connect to RabbitMQ
	conn := rabbitmq.NewRabbitMqConnection(config)

	defer conn.Close()

	channel := rabbitmq.NewRabbitMQChannel(conn)

	// Initialize the services
	runnerService := services.NewRunnerService()
	fileService := services.NewFilesService(config.FileStorageUrl)
	Worker := services.NewWorker(fileService, runnerService)
	queueService := services.NewQueueService(channel, constants.WorkerQueueName, Worker)

	// Start listening for messages
	queueService.Listen()
}
