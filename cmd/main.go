package main

import (
	"context"
	"time"

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
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/languages"
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

	// entrypoint.sh pulls images once at start; keep them current with the weekly runtime rebuilds.
	go docker.RefreshImagesPeriodically(context.Background(), dCli, languages.GetAllDockerImages(),
		constants.RuntimeImageRefreshHours*time.Hour)

	defer func() {
		err := conn.Close()
		if err != nil {
			logger.Error("Failed to close RabbitMQ connection", err)
		}
	}()

	workerChannel := rabbitmq.NewRabbitMQChannel(conn)

	// Initialize the services
	storageService := storage.NewStorage(config.StorageBaseUrl)
	fileCache := storage.NewFileCache(config.CacheDirPath)
	if err := fileCache.InitCache(); err != nil {
		logger.Fatalf("Failed to initialize file cache: %v", err)
	}
	packager := packager.NewPackager(storageService, fileCache)
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
