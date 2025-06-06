package main

import (
	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/internal/config"
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

	defer func() {
		err := conn.Close()
		if err != nil {
			logger.Error("Failed to close RabbitMQ connection", err)
		}
	}()

	workerChannel := rabbitmq.NewRabbitMQChannel(conn)

	// Initialize the services
	dockerService, err := executor.NewDockerExecutor(config.JobsDataVolume)
	if err != nil {
		logger.Fatalf("Failed to initialize Docker service: %s", err.Error())
	}
	runnerService, err := services.NewRunnerService(dockerService)
	if err != nil {
		logger.Fatalf("Failed to initialize runner service: %s", err.Error())
	}
	fileService := services.NewFilesService(config.FileStorageURL)
	workerPool := services.NewWorkerPool(
		workerChannel,
		config.WorkerQueueName,
		config.MaxWorkers,
		fileService,
		runnerService)

	queueService := services.NewQueueService(workerChannel, config.WorkerQueueName, workerPool)

	logger.Info("Listening for messages")
	// Start listening for messages
	queueService.Listen()
}
