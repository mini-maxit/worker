//go:build integration

package rabbitmq_test

import (
	"testing"
	"time"

	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/internal/rabbitmq"
	"github.com/mini-maxit/worker/pkg/constants"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	testRabbitMQURL = "amqp://guest:guest@localhost:5672/"
)

// TestNewRabbitMqConnection_Success tests successful connection to RabbitMQ.
func TestNewRabbitMqConnection_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.Config{
		RabbitMQURL: testRabbitMQURL,
	}

	conn := rabbitmq.NewRabbitMqConnection(cfg)
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	if conn.IsClosed() {
		t.Fatal("connection should not be closed")
	}
}

// TestNewRabbitMQChannel_Success tests creating a channel from connection.
func TestNewRabbitMQChannel_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.Config{
		RabbitMQURL: testRabbitMQURL,
	}

	conn := rabbitmq.NewRabbitMqConnection(cfg)
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	channel := rabbitmq.NewRabbitMQChannel(conn)
	if channel == nil {
		t.Fatal("expected non-nil channel")
	}
}

// TestChannelOperations_QueueDeclare tests basic queue declaration.
func TestChannelOperations_QueueDeclare(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.Config{
		RabbitMQURL: testRabbitMQURL,
	}

	conn := rabbitmq.NewRabbitMqConnection(cfg)
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	channel := rabbitmq.NewRabbitMQChannel(conn)

	queueName := "test_queue_" + time.Now().Format("20060102150405")
	args := make(amqp.Table)
	args["x-max-priority"] = constants.RabbitMQMaxPriority

	queue, err := channel.QueueDeclare(queueName, false, true, false, false, args)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	if queue.Name != queueName {
		t.Fatalf("expected queue name %s, got %s", queueName, queue.Name)
	}
}

// TestChannelOperations_PublishAndConsume tests publishing and consuming messages.
func TestChannelOperations_PublishAndConsume(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.Config{
		RabbitMQURL: testRabbitMQURL,
	}

	conn := rabbitmq.NewRabbitMqConnection(cfg)
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	channel := rabbitmq.NewRabbitMQChannel(conn)

	queueName := "test_pubsub_" + time.Now().Format("20060102150405")

	// Declare queue
	queue, err := channel.QueueDeclare(queueName, false, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Publish a message
	testMessage := "Hello, RabbitMQ!"
	err = channel.Publish("", queue.Name, false, false, amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte(testMessage),
	})
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	// Consume messages
	msgs, err := channel.Consume(queue.Name, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to consume messages: %v", err)
	}

	// Wait for message
	select {
	case msg := <-msgs:
		if string(msg.Body) != testMessage {
			t.Fatalf("expected message %s, got %s", testMessage, string(msg.Body))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}
