package responder

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Responder interface {
	PublishErrorToResponseQueue(
		messageType, messageID string,
		err error,
	)
	PublishSucessHandshakeRespond(
		messageType, messageID string,
		languageSpecs []languages.LanguageSpec,
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

func (r *responder) PublishErrorToResponseQueue(messageType, messageID string, err error) {
	errorPayload := map[string]string{"error": err.Error()}
	payload, jsonErr := json.Marshal(errorPayload)
	if jsonErr != nil {
		r.logger.Errorf("Failed to marshal error payload: %s", jsonErr)
		return
	}

	queueMessage := messages.ResponseQueueMessage{
		Type:      messageType,
		MessageID: messageID,
		Ok:        false,
		Payload:   payload,
	}

	responseJSON, jsonErr := json.Marshal(queueMessage)
	if jsonErr != nil {
		r.logger.Errorf("Failed to marshal response message: %s", jsonErr)
		return
	}

	err = r.channel.Publish("", r.responseQueueName, false, false, amqp.Publishing{
		ContentType:   "application/json",
		CorrelationId: messageID,
		Body:          responseJSON,
	})

	if err != nil {
		r.logger.Errorf("Failed to publish error message: %s", err)
		return
	}

	r.logger.Infof("Published error message to response queue: %s", messageID)
}

func (r *responder) PublishSucessTaskRespond(messageType, messageID string, taskResult solution.Result) error {
	payload, err := json.Marshal(taskResult)
	if err != nil {
		return err
	}

	return r.publishRespondMessage(messageType, messageID, payload)
}

func (r *responder) PublishSucessHandshakeRespond(
	messageType string,
	messageID string,
	languageSpecs []languages.LanguageSpec,
) error {
	// Wrap languages array in the expected HandShakeResponsePayload format
	handshakePayload := struct {
		Languages []languages.LanguageSpec `json:"languages"`
	}{
		Languages: languageSpecs,
	}

	payload, err := json.Marshal(handshakePayload)
	if err != nil {
		return err
	}

	r.logger.Info("payload: ", string(payload))

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

	r.logger.Infof("Publishing response message to response queue: %s", r.responseQueueName)
	err := r.channel.Publish("", r.responseQueueName, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        responseJSON,
	})

	if err != nil {
		return err
	}

	return nil
}
