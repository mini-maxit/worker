package worker

import (
	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)


func NewRabbitMqConnection(config *config.Config) *amqp.Connection {
	logger := logger.NewNamedLogger("rabbitmq")

	logger.Info("Establishing connection to RabbitMQ")
	rabbitMQURL := config.RQUrl

	// Establish connection
	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		logger.Fatalf("Failed to connect to RabbitMQ: %s", err)
	}

	logger.Info("Connection o RabbitMQ established")

	return conn
}

// ConnectToRabbitMQ establishes a connection to RabbitMQ and returns the connection and channel
func NewRabbitMQChannel(conn *amqp.Connection) *amqp.Channel {
	logger := logger.NewNamedLogger("rabbitmq")

	logger.Info("Creating RabbitMQ channel")

	// Create a channel
	ch, err := conn.Channel()
	if err != nil {
		logger.Fatalf("Failed to open a channel: %s", err)
	}

	return ch
}
