package main

import (
	"github.com/docker/docker/client"
	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/rabbitmq"
	"github.com/mini-maxit/worker/internal/rabbitmq/consumer"
	"github.com/mini-maxit/worker/internal/rabbitmq/responder"
	"github.com/mini-maxit/worker/internal/scheduler"
	"github.com/mini-maxit/worker/internal/stages/compiler"
	"github.com/mini-maxit/worker/internal/stages/executor"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/internal/stages/verifier"
	"github.com/mini-maxit/worker/internal/storage"
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

	// Create docker client
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
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
	compiler := compiler.NewCompiler()
	packager := packager.NewPackager(storage)
	executor := executor.NewExecutor(config.JobsDataVolume, cli)
	verifier := verifier.NewVerifier(config.VerifierFlags)
	responder := responder.NewResponder(workerChannel, config.ResponseQueueName)
	scheduler := scheduler.NewScheduler(config.MaxWorkers, compiler, packager, executor, verifier, responder)
	consumer := consumer.NewConsumer(workerChannel, config.ConsumeQueueName, scheduler, responder)

	logger.Info("Listening for messages")
	// Start listening for messages
	consumer.Listen()
}
