package responder

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Responder interface {
	PublishErrorToResponseQueue(
		messageType, messageID string,
		err error,
	) error
	PublishSucessHandshakeRespond(
		messageType, messageID string,
		languageMap map[string][]string,
	) error
	PublishSucessStatusRespond(
		messageType, messageID string,
		statusMap map[string]interface{},
	) error
	PublishSucessTaskRespond(
		messageType, messageID string,
		taskResult solution.Result,
	) error
}

type responder struct {
	logger            *zap.SugaredLogger
	channel           *amqp.Channel
	responseQueueName string
}

func NewResponder(channel *amqp.Channel, responseQueueName string) Responder {
	return &responder{
		logger:            logger.NewNamedLogger("responder"),
		channel:           channel,
		responseQueueName: responseQueueName,
	}
}

func (r *responder) PublishErrorToResponseQueue(messageType, messageID string, err error) error {
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

	err = r.channel.Publish("", r.responseQueueName, false, false, amqp.Publishing{
		ContentType:   "application/json",
		CorrelationId: messageID,
		Body:          responseJSON,
	})

	if err != nil {
		return err
	}

	return nil
}

func (r *responder) PublishSucessTaskRespond(messageType, messageID string, taskResult solution.Result) error {
	payload, err := json.Marshal(taskResult)
	if err != nil {
		return err
	}

	return r.publishRespondMessage(messageType, messageID, payload)
}

func (r *responder) PublishSucessHandshakeRespond(messageType, messageID string, languageMap map[string][]string) error {
	payload, err := json.Marshal(languageMap)
	if err != nil {
		return err
	}

	return r.publishRespondMessage(messageType, messageID, payload)
}

func (r *responder) PublishSucessStatusRespond(messageType, messageID string, statusMap map[string]interface{}) error {
	payload, err := json.Marshal(statusMap)
	if err != nil {
		return err
	}

	return r.publishRespondMessage(messageType, messageID, payload)
}

func (r *responder) publishRespondMessage(messageType, messageID string, payload []byte) error {
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

	err := r.channel.Publish("", r.responseQueueName, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        responseJSON,
	})

	if err != nil {
		return err
	}

	return nil
}