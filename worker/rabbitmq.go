package worker

import (
	"time"

	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

func NewRabbitMqConnection(config *config.Config) *amqp.Connection {
	logger := logger.NewNamedLogger("rabbitmq")

	logger.Info("Establishing connection to RabbitMQ")
	rabbitMQURL := config.RabbitMQUrl

	var err error
	var conn *amqp.Connection

	for v := range 5 {
		conn, err = amqp.Dial(rabbitMQURL)
		if err != nil {
			logger.Warnf("Failed to connect to RabbitMQ: %s", err.Error())
			time.Sleep(2*time.Second + time.Duration(v))
			continue
		}
	}
	if err != nil {
		logger.Panicf("Failed to connect to RabbitMQ: %s", err)
	}

	logger.Info("Connection o RabbitMQ established")

	return conn
}

func NewRabbitMQChannel(conn *amqp.Connection) *amqp.Channel {
	logger := logger.NewNamedLogger("rabbitmq")

	logger.Info("Creating RabbitMQ channel")

	ch, err := conn.Channel()
	if err != nil {
		logger.Panicf("Failed to open a channel: %s", err)
	}

	return ch
}
