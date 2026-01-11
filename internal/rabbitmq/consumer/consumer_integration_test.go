//go:build integration

package consumer_test

import (
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/internal/rabbitmq"
	"github.com/mini-maxit/worker/internal/rabbitmq/consumer"
	"github.com/mini-maxit/worker/internal/rabbitmq/responder"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/tests/mocks"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	testRabbitMQURL = "amqp://guest:guest@localhost:5672/"
)

// TestConsumer_ProcessHandshakeMessage tests processing handshake messages.
func TestConsumer_ProcessHandshakeMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.Config{
		RabbitMQURL: testRabbitMQURL,
	}

	conn := rabbitmq.NewRabbitMqConnection(cfg)
	defer conn.Close()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	channel := rabbitmq.NewRabbitMQChannel(conn)
	resp := responder.NewResponder(channel, constants.DefaultRabbitmqPublishChanSize)
	defer resp.Close()

	mockScheduler := mocks.NewMockScheduler(ctrl)
	workerQueueName := "test_consumer_handshake_" + time.Now().Format("20060102150405")

	cons := consumer.NewConsumer(channel, workerQueueName, mockScheduler, resp)

	// Create a response queue
	responseQueueName := "test_response_handshake_" + time.Now().Format("20060102150405")
	responseQueue, err := channel.QueueDeclare(responseQueueName, false, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare response queue: %v", err)
	}

	// Create a handshake message
	handshakeMessage := messages.QueueMessage{
		Type:      constants.QueueMessageTypeHandshake,
		MessageID: "handshake-msg-id",
		Payload:   json.RawMessage("{}"),
	}

	messageJSON, err := json.Marshal(handshakeMessage)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	// Create a delivery to simulate message consumption
	delivery := amqp.Delivery{
		Body:    messageJSON,
		ReplyTo: responseQueue.Name,
	}

	// Process the message
	cons.ProcessMessage(delivery)

	// Consume the response
	msgs, err := channel.Consume(responseQueue.Name, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to consume messages: %v", err)
	}

	// Wait for response
	select {
	case msg := <-msgs:
		var responseMsg messages.ResponseQueueMessage
		if err := json.Unmarshal(msg.Body, &responseMsg); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if responseMsg.Type != constants.QueueMessageTypeHandshake {
			t.Fatalf("expected type %s, got %s", constants.QueueMessageTypeHandshake, responseMsg.Type)
		}

		if !responseMsg.Ok {
			t.Fatal("expected Ok to be true for handshake response")
		}

		// Verify the payload contains language information
		var handshakePayload struct {
			Languages []languages.LanguageSpec `json:"languages"`
		}
		if err := json.Unmarshal(responseMsg.Payload, &handshakePayload); err != nil {
			t.Fatalf("failed to unmarshal handshake payload: %v", err)
		}

		if len(handshakePayload.Languages) == 0 {
			t.Fatal("expected at least one language in handshake response")
		}

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for handshake response")
	}
}
