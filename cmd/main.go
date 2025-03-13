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

	logger := logger.NewNamedLogger("main")

	logger.Info("Starting worker")

	// Load the configuration
	config := config.NewConfig()

	// Connect to RabbitMQ
	conn := rabbitmq.NewRabbitMqConnection(config)

	defer conn.Close()

	mainChannel := rabbitmq.NewRabbitMQChannel(conn)
	workerChannel := rabbitmq.NewRabbitMQChannel(conn)

	// Initialize the services
	runnerService := services.NewRunnerService()
	fileService := services.NewFilesService(config.FileStorageUrl)
	workerPool := services.NewWorkerPool(workerChannel, constants.WorkerQueueName, constants.MaxWorkers)
	queueService := services.NewQueueService(mainChannel, constants.WorkerQueueName, constants.MainQueueName, fileService, runnerService, workerPool)

	logger.Info("Listening for messages")
	// Start listening for messages
	queueService.Listen()
}
