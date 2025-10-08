package responder

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/pkg/messages"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Responder interface {
	PublishErrorToResponseQueue(
		channel *amqp.Channel,
		responseQueueName, messageType, messageID string,
		err error,
	) error
	PublishSucessToResponseQueue(
		channel *amqp.Channel,
		responseQueueName, messageType, messageID string,
		payload []byte,
	) error
}

type responder struct {
	logger *zap.SugaredLogger
}

func NewResponder() Responder {
	return &responder{
		logger: logger.NewNamedLogger("responder"),
	}
}

func (r *responder)PublishErrorToResponseQueue(
	channel *amqp.Channel,
	responseQueueName, messageType, messageID string,
	err error,
) error {
	errorPayload := map[string]string{"error": err.Error()}
	payload, jsonErr := json.Marshal(errorPayload)
	if jsonErr != nil {
		return jsonErr
	}

	queueMessage := messages.ResponseQueueMessage{
		Type:      messageType,
		MessageID: messageID,
		Ok:        false,
		Payload:   payload,
	}

	responseJSON, jsonErr := json.Marshal(queueMessage)
	if jsonErr != nil {
		return jsonErr
	}

	err = channel.Publish("", responseQueueName, false, false, amqp.Publishing{
		ContentType:   "application/json",
		CorrelationId: messageID,
		Body:          responseJSON,
	})

	if err != nil {
		return err
	}

	return nil
}

func (r *responder) PublishSucessToResponseQueue(
	channel *amqp.Channel,
	responseQueueName, messageType, messageID string,
	payload []byte,
) error {
	queueMessage := messages.ResponseQueueMessage{
		Type:      messageType,
		MessageID: messageID,
		Ok:        true,
		Payload:   payload,
	}

	responseJSON, jsonErr := json.Marshal(queueMessage)
	if jsonErr != nil {
		return jsonErr
	}

	r.logger.Infof("Publishing response to queue: %s", responseQueueName)
	err := channel.Publish("", responseQueueName, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        responseJSON,
	})

	r.logger.Infof("Published response to queue: %s", responseQueueName)

	if err != nil {
		return err
	}

	return nil
}