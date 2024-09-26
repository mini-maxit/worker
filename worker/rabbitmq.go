package worker

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/mini-maxit/worker/utils"
)

// ConnectToRabbitMQ establishes a connection to RabbitMQ and returns the connection and channel
func connectToRabbitMQ() (*amqp.Connection, *amqp.Channel) {
	rabbitMQURL := "amqp://guest:guest@localhost:5672/"

	// Establish connection
	conn, err := amqp.Dial(rabbitMQURL)
	utils.FailOnError(err, "Failed to connect to RabbitMQ")

	// Create a channel
	ch, err := conn.Channel()
	utils.FailOnError(err, "Failed to open a channel")

	return conn, ch
}

