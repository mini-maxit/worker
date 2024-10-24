package main

import (
	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/worker"
)

func main() {
	// Load the configuration
	config := config.LoadWorkerConfig()

	// Connect to the database
	postgresDataBase := worker.NewPostgresDatabase(config)
	db := worker.Connect(postgresDataBase)

	// Connect to RabbitMQ
	conn, ch := worker.NewRabbitMQ(config)

	// Start the worker
	worker.Work(db, conn, ch)
}
