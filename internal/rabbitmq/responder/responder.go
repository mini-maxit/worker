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
		messageType, messageID, respoonsQueue string,
		err error,
	)
	PublishSucessHandshakeRespond(
		messageType, messageID, responseQueue string,
		languageSpecs []languages.LanguageSpec,
	) error
	PublishSucessStatusRespond(
		messageType, messageID, responseQueue string,
		statusMap map[string]interface{},
	) error
	PublishPayloadTaskRespond(
		messageType, messageID, responseQueue string,
		taskResult solution.Result,
	) error
	Publish(queueName string, publishing amqp.Publishing) error
	Close()
}

type publishRequest struct {
	queueName  string
	publishing amqp.Publishing
	done       chan error
}

type responder struct {
	logger               *zap.SugaredLogger
	channel              *amqp.Channel
	defaultResponseQueue string
	publishChan          chan publishRequest
}

func NewResponder(channel *amqp.Channel, responseQueueName string) Responder {
	r := &responder{
		logger:               logger.NewNamedLogger("responder"),
		channel:              channel,
		defaultResponseQueue: responseQueueName,
		publishChan:          make(chan publishRequest, 100),
	}

	go r.publishWorker()
	return r
}

func (r *responder) publishWorker() {
	for req := range r.publishChan {
		err := r.channel.Publish("", req.queueName, false, false, req.publishing)
		req.done <- err
		close(req.done)
	}
}

func (r *responder) Publish(queueName string, publishing amqp.Publishing) error {
	done := make(chan error, 1)
	r.publishChan <- publishRequest{
		queueName:  queueName,
		publishing: publishing,
		done:       done,
	}
	return <-done
}

func (r *responder) Close() {
	close(r.publishChan)
}

func (r *responder) PublishErrorToResponseQueue(messageType, messageID, responseQueue string, err error) {
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

	queueName := r.defaultResponseQueue
	if len(responseQueue) > 0 {
		queueName = responseQueue
	}

	err = r.Publish(queueName, amqp.Publishing{
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

func (r *responder) PublishPayloadTaskRespond(
	messageType string,
	messageID string,
	responseQueue string,
	taskResult solution.Result,
) error {
	payload, err := json.Marshal(taskResult)
	if err != nil {
		return err
	}

	return r.publishRespondMessage(messageType, messageID, responseQueue, payload)
}

func (r *responder) PublishSucessHandshakeRespond(
	messageType string,
	messageID string,
	responseQueue string,
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

	return r.publishRespondMessage(messageType, messageID, responseQueue, payload)
}

func (r *responder) PublishSucessStatusRespond(
	messageType string,
	messageID string,
	responseQueue string,
	statusMap map[string]interface{},
) error {
	payload, err := json.Marshal(statusMap)
	if err != nil {
		return err
	}

	return r.publishRespondMessage(messageType, messageID, responseQueue, payload)
}

func (r *responder) publishRespondMessage(messageType, messageID, responseQueue string, payload []byte) error {
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

	queueName := r.defaultResponseQueue
	if len(responseQueue) > 0 {
		queueName = responseQueue
	}

	r.logger.Infof("Publishing response message to response queue: %s", queueName)
	err := r.Publish(queueName, amqp.Publishing{
		ContentType: "application/json",
		Body:        responseJSON,
	})

	if err != nil {
		return err
	}

	return nil
}
