//go:build integration

package responder_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/internal/rabbitmq"
	"github.com/mini-maxit/worker/internal/rabbitmq/responder"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	testRabbitMQURL = "amqp://guest:guest@localhost:5672/"
)

// setupResponderTest creates a connection, channel, and responder for testing.
func setupResponderTest(t *testing.T) (*amqp.Connection, responder.Responder) {
	cfg := &config.Config{
		RabbitMQURL: testRabbitMQURL,
	}

	conn := rabbitmq.NewRabbitMqConnection(cfg)
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}

	channel := rabbitmq.NewRabbitMQChannel(conn)
	if channel == nil {
		t.Fatal("expected non-nil channel")
	}

	resp := responder.NewResponder(channel, constants.DefaultRabbitmqPublishChanSize)
	if resp == nil {
		t.Fatal("expected non-nil responder")
	}

	return conn, resp
}

// TestNewResponder_Success tests creating a new responder.
func TestNewResponder_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	conn, resp := setupResponderTest(t)
	defer func() {
		if err := resp.Close(); err != nil {
			t.Logf("failed to close responder: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	// Test that responder was created successfully
	if resp == nil {
		t.Fatal("responder should not be nil")
	}
}

// TestResponder_Publish tests publishing a message to a queue.
func TestResponder_Publish(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	conn, resp := setupResponderTest(t)
	defer func() {
		if err := resp.Close(); err != nil {
			t.Logf("failed to close responder: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	// Create a test queue
	channel := rabbitmq.NewRabbitMQChannel(conn)
	queueName := "test_publish_" + time.Now().Format("20060102150405")
	queue, err := channel.QueueDeclare(queueName, false, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Publish a message
	testMessage := "test message"
	err = resp.Publish(queue.Name, amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte(testMessage),
	})
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	// Verify message was published by consuming it
	msgs, err := channel.Consume(queue.Name, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to consume messages: %v", err)
	}

	select {
	case msg := <-msgs:
		if string(msg.Body) != testMessage {
			t.Fatalf("expected message %s, got %s", testMessage, string(msg.Body))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

// TestResponder_PublishMultiple tests publishing multiple messages.
func TestResponder_PublishMultiple(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	conn, resp := setupResponderTest(t)
	defer func() {
		if err := resp.Close(); err != nil {
			t.Logf("failed to close responder: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	// Create a test queue
	channel := rabbitmq.NewRabbitMQChannel(conn)
	queueName := "test_publish_multi_" + time.Now().Format("20060102150405")
	queue, err := channel.QueueDeclare(queueName, false, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Publish multiple messages
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		err = resp.Publish(queue.Name, amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte("Message " + string(rune(i+'0'))),
		})
		if err != nil {
			t.Fatalf("failed to publish message %d: %v", i, err)
		}
	}

	// Verify all messages were published
	msgs, err := channel.Consume(queue.Name, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to consume messages: %v", err)
	}

	received := 0
	timeout := time.After(10 * time.Second)

	for received < messageCount {
		select {
		case <-msgs:
			received++
		case <-timeout:
			t.Fatalf("timeout waiting for messages, received %d out of %d", received, messageCount)
		}
	}
}

// TestResponder_PublishErrorToResponseQueue tests publishing error messages.
func TestResponder_PublishErrorToResponseQueue(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	conn, resp := setupResponderTest(t)
	defer func() {
		if err := resp.Close(); err != nil {
			t.Logf("failed to close responder: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	// Create a test response queue
	channel := rabbitmq.NewRabbitMQChannel(conn)
	queueName := "test_error_response_" + time.Now().Format("20060102150405")
	queue, err := channel.QueueDeclare(queueName, false, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Publish an error message
	messageType := constants.QueueMessageTypeTask
	messageID := "test-message-id"
	testError := errors.ErrInvalidLanguageType

	resp.PublishErrorToResponseQueue(messageType, messageID, queue.Name, testError)

	// Consume and verify the error message
	msgs, err := channel.Consume(queue.Name, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to consume messages: %v", err)
	}

	select {
	case msg := <-msgs:
		var responseMsg messages.ResponseQueueMessage
		if err := json.Unmarshal(msg.Body, &responseMsg); err != nil {
			t.Fatalf("failed to unmarshal response message: %v", err)
		}

		if responseMsg.Type != messageType {
			t.Fatalf("expected message type %s, got %s", messageType, responseMsg.Type)
		}
		if responseMsg.MessageID != messageID {
			t.Fatalf("expected message id %s, got %s", messageID, responseMsg.MessageID)
		}
		if responseMsg.Ok {
			t.Fatal("expected Ok to be false for error message")
		}

		var errorPayload map[string]string
		if err := json.Unmarshal(responseMsg.Payload, &errorPayload); err != nil {
			t.Fatalf("failed to unmarshal error payload: %v", err)
		}

		if errorPayload["error"] != testError.Error() {
			t.Fatalf("expected error %s, got %s", testError.Error(), errorPayload["error"])
		}

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for error message")
	}
}

// TestResponder_PublishSuccessHandshakeRespond tests publishing handshake response.
func TestResponder_PublishSuccessHandshakeRespond(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	conn, resp := setupResponderTest(t)
	defer func() {
		if err := resp.Close(); err != nil {
			t.Logf("failed to close responder: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	// Create a test response queue
	channel := rabbitmq.NewRabbitMQChannel(conn)
	queueName := "test_handshake_" + time.Now().Format("20060102150405")
	queue, err := channel.QueueDeclare(queueName, false, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Create test language specs
	languageSpecs := []languages.LanguageSpec{
		{LanguageName: "cpp", Versions: []string{"17"}, Extension: "cpp"},
		{LanguageName: "python", Versions: []string{"3.11"}, Extension: "py"},
	}

	messageType := constants.QueueMessageTypeHandshake
	messageID := "handshake-message-id"

	// Publish handshake response
	err = resp.PublishSuccessHandshakeRespond(messageType, messageID, queue.Name, languageSpecs)
	if err != nil {
		t.Fatalf("failed to publish handshake response: %v", err)
	}

	// Consume and verify the response
	msgs, err := channel.Consume(queue.Name, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to consume messages: %v", err)
	}

	select {
	case msg := <-msgs:
		var responseMsg messages.ResponseQueueMessage
		if err := json.Unmarshal(msg.Body, &responseMsg); err != nil {
			t.Fatalf("failed to unmarshal response message: %v", err)
		}

		if responseMsg.Type != messageType {
			t.Fatalf("expected message type %s, got %s", messageType, responseMsg.Type)
		}
		if responseMsg.MessageID != messageID {
			t.Fatalf("expected message id %s, got %s", messageID, responseMsg.MessageID)
		}
		if !responseMsg.Ok {
			t.Fatal("expected Ok to be true for success message")
		}

		var handshakePayload struct {
			Languages []languages.LanguageSpec `json:"languages"`
		}
		if err := json.Unmarshal(responseMsg.Payload, &handshakePayload); err != nil {
			t.Fatalf("failed to unmarshal handshake payload: %v", err)
		}

		if len(handshakePayload.Languages) != len(languageSpecs) {
			t.Fatalf("expected %d languages, got %d", len(languageSpecs), len(handshakePayload.Languages))
		}

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for handshake response")
	}
}

// TestResponder_PublishSuccessStatusRespond tests publishing status response.
func TestResponder_PublishSuccessStatusRespond(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	conn, resp := setupResponderTest(t)
	defer func() {
		if err := resp.Close(); err != nil {
			t.Logf("failed to close responder: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	// Create a test response queue
	channel := rabbitmq.NewRabbitMQChannel(conn)
	queueName := "test_status_" + time.Now().Format("20060102150405")
	queue, err := channel.QueueDeclare(queueName, false, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Create test status
	status := messages.ResponseWorkerStatusPayload{
		BusyWorkers:  2,
		TotalWorkers: 5,
		WorkerStatus: []messages.WorkerStatus{
			{WorkerID: 1, Status: constants.WorkerStatusBusy, ProcessingMessageID: "msg-1"},
			{WorkerID: 2, Status: constants.WorkerStatusIdle, ProcessingMessageID: ""},
		},
	}

	messageType := constants.QueueMessageTypeStatus
	messageID := "status-message-id"

	// Publish status response
	err = resp.PublishSuccessStatusRespond(messageType, messageID, queue.Name, status)
	if err != nil {
		t.Fatalf("failed to publish status response: %v", err)
	}

	// Consume and verify the response
	msgs, err := channel.Consume(queue.Name, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to consume messages: %v", err)
	}

	select {
	case msg := <-msgs:
		var responseMsg messages.ResponseQueueMessage
		if err := json.Unmarshal(msg.Body, &responseMsg); err != nil {
			t.Fatalf("failed to unmarshal response message: %v", err)
		}

		if responseMsg.Type != messageType {
			t.Fatalf("expected message type %s, got %s", messageType, responseMsg.Type)
		}
		if responseMsg.MessageID != messageID {
			t.Fatalf("expected message id %s, got %s", messageID, responseMsg.MessageID)
		}
		if !responseMsg.Ok {
			t.Fatal("expected Ok to be true for success message")
		}

		var statusPayload messages.ResponseWorkerStatusPayload
		if err := json.Unmarshal(responseMsg.Payload, &statusPayload); err != nil {
			t.Fatalf("failed to unmarshal status payload: %v", err)
		}

		if statusPayload.BusyWorkers != status.BusyWorkers {
			t.Fatalf("expected busy workers %d, got %d", status.BusyWorkers, statusPayload.BusyWorkers)
		}
		if statusPayload.TotalWorkers != status.TotalWorkers {
			t.Fatalf("expected total workers %d, got %d", status.TotalWorkers, statusPayload.TotalWorkers)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for status response")
	}
}

// TestResponder_PublishPayloadTaskRespond tests publishing task result response.
func TestResponder_PublishPayloadTaskRespond(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	conn, resp := setupResponderTest(t)
	defer func() {
		if err := resp.Close(); err != nil {
			t.Logf("failed to close responder: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	// Create a test response queue
	channel := rabbitmq.NewRabbitMQChannel(conn)
	queueName := "test_task_result_" + time.Now().Format("20060102150405")
	queue, err := channel.QueueDeclare(queueName, false, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Create test result
	taskResult := solution.Result{
		StatusCode: solution.Success,
		Message:    constants.SolutionMessageSuccess,
		TestResults: []solution.TestResult{
			{
				Order:         1,
				StatusCode:    solution.TestCasePassed,
				Passed:        true,
				ExecutionTime: 100.5,
				ErrorMessage:  "",
			},
		},
	}

	messageType := constants.QueueMessageTypeTask
	messageID := "task-result-message-id"

	// Publish task result response
	err = resp.PublishPayloadTaskRespond(messageType, messageID, queue.Name, taskResult)
	if err != nil {
		t.Fatalf("failed to publish task result response: %v", err)
	}

	// Consume and verify the response
	msgs, err := channel.Consume(queue.Name, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to consume messages: %v", err)
	}

	select {
	case msg := <-msgs:
		var responseMsg messages.ResponseQueueMessage
		if err := json.Unmarshal(msg.Body, &responseMsg); err != nil {
			t.Fatalf("failed to unmarshal response message: %v", err)
		}

		if responseMsg.Type != messageType {
			t.Fatalf("expected message type %s, got %s", messageType, responseMsg.Type)
		}
		if responseMsg.MessageID != messageID {
			t.Fatalf("expected message id %s, got %s", messageID, responseMsg.MessageID)
		}
		if !responseMsg.Ok {
			t.Fatal("expected Ok to be true for success message")
		}

		var resultPayload solution.Result
		if err := json.Unmarshal(responseMsg.Payload, &resultPayload); err != nil {
			t.Fatalf("failed to unmarshal result payload: %v", err)
		}

		if resultPayload.Message != taskResult.Message {
			t.Fatalf("expected message %s, got %s", taskResult.Message, resultPayload.Message)
		}
		if resultPayload.StatusCode != taskResult.StatusCode {
			t.Fatalf("expected status code %d, got %d", taskResult.StatusCode, resultPayload.StatusCode)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for task result response")
	}
}

// TestResponder_Close tests closing the responder.
func TestResponder_Close(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	conn, resp := setupResponderTest(t)
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	// Close the responder
	err := resp.Close()
	if err != nil {
		t.Fatalf("failed to close responder: %v", err)
	}

	// Try to close again - should return error
	err = resp.Close()
	if err != errors.ErrResponderClosed {
		t.Fatalf("expected ErrResponderClosed, got %v", err)
	}
}

// TestResponder_PublishAfterClose tests publishing after closing responder.
func TestResponder_PublishAfterClose(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	conn, resp := setupResponderTest(t)
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	// Close the responder
	err := resp.Close()
	if err != nil {
		t.Fatalf("failed to close responder: %v", err)
	}

	// Try to publish after closing
	err = resp.Publish("test_queue", amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte("test"),
	})

	if err != errors.ErrResponderClosed {
		t.Fatalf("expected ErrResponderClosed, got %v", err)
	}
}

// TestResponder_ConcurrentPublish tests concurrent publishing.
func TestResponder_ConcurrentPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	conn, resp := setupResponderTest(t)
	defer func() {
		if err := resp.Close(); err != nil {
			t.Logf("failed to close responder: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Logf("failed to close connection: %v", err)
		}
	}()

	// Create a test queue
	channel := rabbitmq.NewRabbitMQChannel(conn)
	queueName := "test_concurrent_" + time.Now().Format("20060102150405")
	queue, err := channel.QueueDeclare(queueName, false, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Publish messages concurrently
	goroutineCount := 10
	messagesPerGoroutine := 5
	totalMessages := goroutineCount * messagesPerGoroutine

	done := make(chan bool, goroutineCount)
	for i := 0; i < goroutineCount; i++ {
		go func(id int) {
			for j := 0; j < messagesPerGoroutine; j++ {
				err := resp.Publish(queue.Name, amqp.Publishing{
					ContentType: "text/plain",
					Body:        []byte("Message from goroutine " + string(rune(id+'0'))),
				})
				if err != nil {
					t.Errorf("failed to publish message: %v", err)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to finish
	for i := 0; i < goroutineCount; i++ {
		<-done
	}

	// Verify all messages were published
	msgs, err := channel.Consume(queue.Name, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("failed to consume messages: %v", err)
	}

	received := 0
	timeout := time.After(15 * time.Second)

	for received < totalMessages {
		select {
		case <-msgs:
			received++
		case <-timeout:
			t.Fatalf("timeout waiting for messages, received %d out of %d", received, totalMessages)
		}
	}

	if received != totalMessages {
		t.Fatalf("expected %d messages, got %d", totalMessages, received)
	}
}
