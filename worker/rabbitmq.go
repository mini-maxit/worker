package worker

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/mini-maxit/worker/utils"
	"github.com/mini-maxit/worker/internal/config"
)

// ConnectToRabbitMQ establishes a connection to RabbitMQ and returns the connection and channel
func NewRabbitMQ(config config.Config) (*amqp.Connection, *amqp.Channel) {
	rabbitMQURL := config.RQUrl

	// Establish connection
	conn, err := amqp.Dial(rabbitMQURL)
	utils.CheckError(err, "Failed to connect to RabbitMQ")

	// Create a channel
	ch, err := conn.Channel()
	utils.CheckError(err, "Failed to open a channel")

	return conn, ch
}

