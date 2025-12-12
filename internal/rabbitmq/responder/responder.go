package responder

import (
	"encoding/json"
	"sync"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/rabbitmq/channel"
	"github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Responder interface {
	PublishErrorToResponseQueue(
		messageType, messageID, responseQueue string,
		err error,
	)
	PublishTaskErrorToResponseQueue(
		messageType, messageID, responseQueue string,
		err error,
	)
	PublishSuccessHandshakeRespond(
		messageType, messageID, responseQueue string,
		languageSpecs messages.ResponseHandshakePayload,
	)
	PublishSuccessStatusRespond(
		messageType, messageID, responseQueue string,
		status messages.ResponseWorkerStatusPayload,
	)
	PublishPayloadTaskRespond(
		messageType, messageID, responseQueue string,
		taskResult solution.Result,
	)
	Publish(queueName string, publishing amqp.Publishing) error
	Close() error
}

type publishRequest struct {
	queueName  string
	publishing amqp.Publishing
	done       chan error
}

type responder struct {
	logger      *zap.SugaredLogger
	channel     channel.Channel
	publishChan chan publishRequest
	mu          sync.Mutex
	closed      bool
}

func NewResponder(ch channel.Channel, publishChanSize int) Responder {
	r := &responder{
		logger:      logger.NewNamedLogger("responder"),
		channel:     ch,
		publishChan: make(chan publishRequest, publishChanSize),
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

func (r *responder) Publish(queueName string, publishing amqp.Publishing) (err error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return errors.ErrResponderClosed
	}
	r.mu.Unlock()

	done := make(chan error, 1)

	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Errorf("panic in Publish: %v", rec)
			err = errors.ErrResponderClosed
		}
	}()

	r.publishChan <- publishRequest{
		queueName:  queueName,
		publishing: publishing,
		done:       done,
	}
	return <-done
}

func (r *responder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return errors.ErrResponderClosed
	}

	r.closed = true
	close(r.publishChan)
	return nil
}

func (r *responder) PublishTaskErrorToResponseQueue(messageType, messageID, responseQueue string, err error) {
	internalErrorTaskResult := solution.Result{
		StatusCode:  solution.InternalError,
		Message:     err.Error(),
		TestResults: []solution.TestResult{},
	}

	payload, err := json.Marshal(internalErrorTaskResult)
	if err != nil {
		r.logger.Errorf("Failed to marshal error payload: %s", err)
		return
	}

	r.publishRespondMessage(messageType, messageID, true, responseQueue, payload)
}

func (r *responder) PublishErrorToResponseQueue(messageType, messageID, responseQueue string, err error) {
	payload, jsonErr := json.Marshal(messages.ResponseErrorPayload{
		Error: err.Error(),
	})
	if jsonErr != nil {
		r.logger.Errorf("Failed to marshal error payload: %s", jsonErr)
		return
	}

	r.publishRespondMessage(messageType, messageID, false, responseQueue, payload)
}

func (r *responder) PublishPayloadTaskRespond(
	messageType string,
	messageID string,
	responseQueue string,
	taskResult solution.Result,
) {
	payload, err := json.Marshal(taskResult)
	if err != nil {
		r.logger.Errorf("Failed to marshal task result payload: %s", err)
		return
	}

	r.publishRespondMessage(messageType, messageID, true, responseQueue, payload)
}

func (r *responder) PublishSuccessHandshakeRespond(
	messageType string,
	messageID string,
	responseQueue string,
	languageSpecs messages.ResponseHandshakePayload,
) {
	payload, err := json.Marshal(languageSpecs)
	if err != nil {
		r.logger.Errorf("Failed to marshal handshake payload: %s", err)
		return
	}

	r.publishRespondMessage(messageType, messageID, true, responseQueue, payload)
}

func (r *responder) PublishSuccessStatusRespond(
	messageType string,
	messageID string,
	responseQueue string,
	status messages.ResponseWorkerStatusPayload,
) {
	payload, err := json.Marshal(status)
	if err != nil {
		r.logger.Errorf("Failed to marshal status payload: %s", err)
		return
	}

	r.publishRespondMessage(messageType, messageID, true, responseQueue, payload)
}

func (r *responder) publishRespondMessage(
	messageType, messageID string,
	ok bool,
	responseQueue string,
	payload []byte,
) {
	queueMessage := messages.ResponseQueueMessage{
		Type:      messageType,
		MessageID: messageID,
		Ok:        ok,
		Payload:   payload,
	}

	responseJSON, jsonErr := json.Marshal(queueMessage)
	if jsonErr != nil {
		r.logger.Errorf("Failed to marshal response message: %s", jsonErr)
		return
	}

	err := r.Publish(responseQueue, amqp.Publishing{
		ContentType: "application/json",
		Body:        responseJSON,
	})

	if err != nil {
		r.logger.Errorf("Failed to publish response message: %s", err)
		return
	}

	r.logger.Infof("Published response message to response queue: %s", messageID)
}
