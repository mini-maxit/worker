package worker

import (
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ConnectToRabbitMQ establishes a connection to RabbitMQ and returns the connection and channel
func connectToRabbitMQ() (*amqp.Connection, *amqp.Channel) {
	rabbitMQURL := "amqp://guest:guest@localhost:5672/"

	// Establish connection
	conn, err := amqp.Dial(rabbitMQURL)
	failOnError(err, "Failed to connect to RabbitMQ")

	// Create a channel
	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")

	return conn, ch
}

// Helper function to log errors
func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}