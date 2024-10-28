package worker

import (
	"log"

	"github.com/mini-maxit/worker/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ConnectToRabbitMQ establishes a connection to RabbitMQ and returns the connection and channel
func NewRabbitMQ(config config.Config) (*amqp.Connection, *amqp.Channel) {
	rabbitMQURL := config.RQUrl

	// Establish connection
	conn, err := amqp.Dial(rabbitMQURL)
	if(err != nil) {
		log.Fatalf("Failed to connect to RabbitMQ: %s", err)
	}

	// Create a channel
	ch, err := conn.Channel()
	if(err != nil) {
		log.Fatalf("Failed to open a channel: %s", err)
	}

	return conn, ch
}
