package main

import (
	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/internal/docker"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/rabbitmq"
	"github.com/mini-maxit/worker/internal/rabbitmq/consumer"
	"github.com/mini-maxit/worker/internal/rabbitmq/responder"
	"github.com/mini-maxit/worker/internal/scheduler"
	"github.com/mini-maxit/worker/internal/stages/executor"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/internal/stages/verifier"
	"github.com/mini-maxit/worker/internal/storage"
)

func main() {
	logger := logger.NewNamedLogger("main")

	logger.Info("Starting worker")

	// Load the configuration
	config := config.NewConfig()

	// Connect to RabbitMQ
	conn := rabbitmq.NewRabbitMqConnection(config)

	// Create docker client
	dCli, err := docker.NewDockerClient()
	if err != nil {
		logger.Fatalf("Failed to create Docker client: %s", err.Error())
	}

	defer func() {
		err := conn.Close()
		if err != nil {
			logger.Error("Failed to close RabbitMQ connection", err)
		}
	}()

	workerChannel := rabbitmq.NewRabbitMQChannel(conn)

	// Initialize the services
	storage := storage.NewStorage(config.StorageBaseUrl)
	packager := packager.NewPackager(storage)
	executor := executor.NewExecutor(dCli)
	verifier := verifier.NewVerifier(config.VerifierFlags)
	responder := responder.NewResponder(workerChannel, config.PublishChanSize)
	defer func() {
		if err := responder.Close(); err != nil {
			logger.Error("Failed to close responder", err)
		}
	}()
	scheduler := scheduler.NewScheduler(config.MaxWorkers, packager, executor, verifier, responder)
	consumer := consumer.NewConsumer(workerChannel, config.ConsumeQueueName, scheduler, responder)

	// Start listening for messages
	consumer.Listen()
}
