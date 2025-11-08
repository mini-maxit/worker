package consumer

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/rabbitmq/channel"
	"github.com/mini-maxit/worker/internal/rabbitmq/responder"
	"github.com/mini-maxit/worker/internal/scheduler"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	e "errors"
)

type Consumer interface {
	Listen()
}

type consumer struct {
	channel         channel.Channel
	workerQueueName string
	scheduler       scheduler.Scheduler
	responder       responder.Responder
	logger          *zap.SugaredLogger
}

func NewConsumer(
	mainChannel channel.Channel,
	workerQueueName string,
	scheduler scheduler.Scheduler,
	responder responder.Responder,
) Consumer {
	logger := logger.NewNamedLogger("consumer")

	return &consumer{
		channel:         mainChannel,
		workerQueueName: workerQueueName,
		scheduler:       scheduler,
		responder:       responder,
		logger:          logger,
	}
}

func (c *consumer) Listen() {
	c.logger.Infof("Declaring queue %s", c.workerQueueName)

	args := make(amqp.Table)
	args["x-max-priority"] = constants.RabbitMQMaxPriority
	_, err := c.channel.QueueDeclare(c.workerQueueName, true, false, false, false, args)
	if err != nil {
		c.logger.Panicf("Failed to declare queue %s: %s", c.workerQueueName, err)
	}

	c.logger.Infof("Listening for messages on queue %s", c.workerQueueName)

	msgs, err := c.channel.Consume(c.workerQueueName, "", true, false, false, false, nil)
	if err != nil {
		c.logger.Panicf("Failed to consume messages from queue %s: %s", c.workerQueueName, err)
	}

	for msg := range msgs {
		go func(m amqp.Delivery) {
			c.processMessage(m)
		}(msg)
	}
}

func (c *consumer) processMessage(msg amqp.Delivery) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Errorf("Panic while processing message: %v", r)
		}
	}()
	var queueMessage messages.QueueMessage

	err := json.Unmarshal(msg.Body, &queueMessage)
	if err != nil {
		c.logger.Errorf("Failed to unmarshal message: %s", err)
		c.responder.PublishErrorToResponseQueue(queueMessage.Type, queueMessage.MessageID, msg.ReplyTo, err)
		return
	}

	switch queueMessage.Type {
	case constants.QueueMessageTypeTask:
		c.logger.Infof("Received task message: %s", queueMessage.MessageID)
		c.handleTaskMessage(queueMessage, msg.ReplyTo)
	case constants.QueueMessageTypeStatus:
		c.logger.Infof("Received status message: %s", queueMessage.MessageID)
		c.handleStatusMessage(queueMessage, msg.ReplyTo)
	case constants.QueueMessageTypeHandshake:
		c.logger.Infof("Received handshake message: %s", queueMessage.MessageID)
		c.handleHandshakeMessage(queueMessage, msg.ReplyTo)
	default:
		c.logger.Errorf("Unknown message type: %s", queueMessage.Type)
		c.responder.PublishErrorToResponseQueue(
			queueMessage.Type,
			queueMessage.MessageID,
			msg.ReplyTo,
			errors.ErrUnknownMessageType)
	}
}

func (c *consumer) requeueTaskWithPriority2(queueMessage messages.QueueMessage) error {
	priority := constants.RabbitMQRequeuePriority

	queueMessageJSON, err := json.Marshal(queueMessage)
	if err != nil {
		c.logger.Errorf("Failed to marshal queue message: %s", err)
		return err
	}

	err = c.responder.Publish(c.workerQueueName, amqp.Publishing{
		ContentType: "application/json",
		Body:        queueMessageJSON,
		Priority:    uint8(priority),
	})

	if err != nil {
		c.logger.Errorf("Failed to requeue task with higher priority: %s", err)
		return err
	}

	return nil
}

func (c *consumer) handleTaskMessage(queueMessage messages.QueueMessage, replyTo string) {
	c.logger.Infof("Processing task message")

	var task *messages.TaskQueueMessage
	if err := json.Unmarshal(queueMessage.Payload, &task); err != nil {
		c.logger.Errorf("Failed to unmarshal task message: %s", err)
		c.responder.PublishErrorToResponseQueue(
			queueMessage.Type,
			queueMessage.MessageID,
			replyTo,
			err)
		return
	}

	err := c.scheduler.ProcessTask(replyTo, queueMessage.MessageID, task)
	if err == nil {
		return
	}

	c.logger.Errorf("Failed to process task message: %s", err)

	if e.Is(err, errors.ErrFailedToGetFreeWorker) {
		requeueErr := c.requeueTaskWithPriority2(queueMessage)
		if requeueErr != nil {
			c.logger.Errorf("Failed to requeue task with higher priority: %s", requeueErr)
		}
		return
	}

	c.responder.PublishErrorToResponseQueue(
		queueMessage.Type,
		queueMessage.MessageID,
		replyTo,
		err)
}

func (c *consumer) handleStatusMessage(queueMessage messages.QueueMessage, replyTo string) {
	c.logger.Infof("Processing status message")
	status := c.scheduler.GetWorkersStatus()

	err := c.responder.PublishSucessStatusRespond(queueMessage.Type, queueMessage.MessageID, replyTo, status)
	if err != nil {
		c.logger.Errorf("Failed to publish status message: %s", err)
		c.responder.PublishErrorToResponseQueue(queueMessage.Type, queueMessage.MessageID, replyTo, err)
	}
}

func (c *consumer) handleHandshakeMessage(queueMessage messages.QueueMessage, replyTo string) {
	c.logger.Infof("Processing handshake message")
	languages := languages.GetSupportedLanguagesWithVersions()

	err := c.responder.PublishSucessHandshakeRespond(queueMessage.Type, queueMessage.MessageID, replyTo, languages)
	if err != nil {
		c.logger.Errorf("Failed to publish supported languages: %s", err)
		c.responder.PublishErrorToResponseQueue(queueMessage.Type, queueMessage.MessageID, replyTo, err)
	}
}
