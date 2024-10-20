package worker

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/mini-maxit/worker/utils"
)

// ConnectToRabbitMQ establishes a connection to RabbitMQ and returns the connection and channel
func connectToRabbitMQ() (*amqp.Connection, *amqp.Channel) {
	Config := LoadConfig()
	rabbitMQURL := Config.RQUrl

	// Establish connection
	conn, err := amqp.Dial(rabbitMQURL)
	utils.CheckError(err, "Failed to connect to RabbitMQ")

	// Create a channel
	ch, err := conn.Channel()
	utils.CheckError(err, "Failed to open a channel")

	return conn, ch
}

