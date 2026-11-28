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
	ProcessMessage(msg amqp.Delivery)
	ProcessTaskDelivery(msg amqp.Delivery)
}

type consumer struct {
	channel         channel.Channel
	workerQueueName string
	taskQueueName   string
	maxWorkers      int
	scheduler       scheduler.Scheduler
	responder       responder.Responder
	logger          *zap.SugaredLogger
}

func NewConsumer(
	mainChannel channel.Channel,
	workerQueueName string,
	maxWorkers int,
	scheduler scheduler.Scheduler,
	responder responder.Responder,
) Consumer {
	logger := logger.NewNamedLogger("consumer")

	return &consumer{
		channel:         mainChannel,
		workerQueueName: workerQueueName,
		taskQueueName:   workerQueueName + constants.TaskQueueSuffix,
		maxWorkers:      maxWorkers,
		scheduler:       scheduler,
		responder:       responder,
		logger:          logger,
	}
}

// Listen consumes two queues. The worker queue takes every message from the backend and is never limited,
// so status and handshake messages are answered while all workers are busy. Tasks are moved from it to the
// task queue, which hands out at most maxWorkers unacknowledged tasks.
func (c *consumer) Listen() {
	c.logger.Infof("Declaring queue %s", c.workerQueueName)

	args := make(amqp.Table)
	args["x-max-priority"] = constants.RabbitMQMaxPriority
	_, err := c.channel.QueueDeclare(c.workerQueueName, true, false, false, false, args)
	if err != nil {
		c.logger.Panicf("Failed to declare queue %s: %s", c.workerQueueName, err)
	}

	c.logger.Infof("Declaring queue %s", c.taskQueueName)
	_, err = c.channel.QueueDeclare(c.taskQueueName, true, false, false, false, args)
	if err != nil {
		c.logger.Panicf("Failed to declare queue %s: %s", c.taskQueueName, err)
	}

	c.logger.Infof("Listening for messages on queue %s", c.workerQueueName)

	msgs, err := c.channel.Consume(c.workerQueueName, "", false, false, false, false, nil)
	if err != nil {
		c.logger.Panicf("Failed to consume messages from queue %s: %s", c.workerQueueName, err)
	}

	// A per-consumer limit applies only to consumers started after this call, so it must stay between the two.
	if err := c.channel.Qos(c.maxWorkers, 0, false); err != nil {
		c.logger.Panicf("Failed to set prefetch count: %s", err)
	}

	tasks, err := c.channel.Consume(c.taskQueueName, "", false, false, false, false, nil)
	if err != nil {
		c.logger.Panicf("Failed to consume messages from queue %s: %s", c.taskQueueName, err)
	}

	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			// Concurrent, so a slow reply cannot hold up the rest of the worker queue.
			go c.ProcessMessage(msg)
		case msg, ok := <-tasks:
			if !ok {
				return
			}
			c.ProcessTaskDelivery(msg)
		}
	}
}

// ProcessMessage handles a delivery from the worker queue.
func (c *consumer) ProcessMessage(msg amqp.Delivery) {
	defer c.dropOnPanic(msg)
	var queueMessage messages.QueueMessage

	err := json.Unmarshal(msg.Body, &queueMessage)
	if err != nil {
		c.logger.Errorf("Failed to unmarshal message: %s", err)
		c.responder.PublishErrorToResponseQueue(queueMessage.Type, queueMessage.MessageID, msg.ReplyTo, err)
		c.ack(msg)
		return
	}

	switch queueMessage.Type {
	case constants.QueueMessageTypeTask:
		c.logger.Infof("Received task message [MsgID: %s]", queueMessage.MessageID)
		if err := c.enqueueTask(msg); err != nil {
			c.logger.Errorf("Failed to enqueue task %s: %s", queueMessage.MessageID, err)
			c.requeue(msg)
			return
		}
	case constants.QueueMessageTypeStatus:
		c.logger.Infof("Received status message [MsgID: %s]", queueMessage.MessageID)
		c.handleStatusMessage(queueMessage, msg.ReplyTo)
	case constants.QueueMessageTypeHandshake:
		c.logger.Infof("Received handshake message [MsgID: %s]", queueMessage.MessageID)
		c.handleHandshakeMessage(queueMessage, msg.ReplyTo)
	default:
		c.logger.Errorf("Unknown message type: %s", queueMessage.Type)
		c.responder.PublishErrorToResponseQueue(
			queueMessage.Type,
			queueMessage.MessageID,
			msg.ReplyTo,
			errors.ErrUnknownMessageType)
	}
	c.ack(msg)
}

// ProcessTaskDelivery handles a delivery from the task queue. It is acknowledged once its response is published.
func (c *consumer) ProcessTaskDelivery(msg amqp.Delivery) {
	defer c.dropOnPanic(msg)

	var queueMessage messages.QueueMessage
	var task *messages.TaskQueueMessage
	err := json.Unmarshal(msg.Body, &queueMessage)
	if err == nil {
		err = json.Unmarshal(queueMessage.Payload, &task)
	}
	if err != nil {
		c.logger.Errorf("Failed to unmarshal task message: %s", err)
		c.responder.PublishErrorToResponseQueue(queueMessage.Type, queueMessage.MessageID, msg.ReplyTo, err)
		c.ack(msg)
		return
	}

	err = c.scheduler.ProcessTask(msg.ReplyTo, queueMessage.MessageID, task, func(taskErr error) {
		if e.Is(taskErr, errors.ErrWorkerPanicked) {
			// Redelivering would panic again.
			c.responder.PublishErrorToResponseQueue(queueMessage.Type, queueMessage.MessageID, msg.ReplyTo, taskErr)
			c.ack(msg)
			return
		}
		if taskErr != nil {
			c.logger.Errorf("No response published for task %s: %s", queueMessage.MessageID, taskErr)
			c.requeue(msg)
			return
		}
		c.ack(msg)
	})
	if err == nil {
		return
	}

	if e.Is(err, errors.ErrFailedToGetFreeWorker) {
		c.logger.Warnf("Requeuing task %s due to no available workers", queueMessage.MessageID)
		c.requeue(msg)
		return
	}

	c.responder.PublishErrorToResponseQueue(
		queueMessage.Type,
		queueMessage.MessageID,
		msg.ReplyTo,
		err)
	c.ack(msg)
}

// enqueueTask copies a task to the task queue. The original is acknowledged afterwards on the same channel,
// so the broker receives the copy first.
func (c *consumer) enqueueTask(msg amqp.Delivery) error {
	return c.responder.Publish(c.taskQueueName, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		ReplyTo:      msg.ReplyTo,
		Priority:     msg.Priority,
		Body:         msg.Body,
	})
}

func (c *consumer) ack(msg amqp.Delivery) {
	if err := msg.Ack(false); err != nil {
		c.logger.Errorf("Failed to ack message: %s", err)
	}
}

func (c *consumer) requeue(msg amqp.Delivery) {
	if err := msg.Nack(false, true); err != nil {
		c.logger.Errorf("Failed to requeue message: %s", err)
	}
}

// dropOnPanic discards a message whose handling panicked, because redelivering it would panic again.
func (c *consumer) dropOnPanic(msg amqp.Delivery) {
	if r := recover(); r != nil {
		c.logger.Errorf("Panic while processing message: %v", r)
		if err := msg.Nack(false, false); err != nil {
			c.logger.Errorf("Failed to drop message: %s", err)
		}
	}
}

func (c *consumer) handleStatusMessage(queueMessage messages.QueueMessage, replyTo string) {
	status := c.scheduler.GetWorkersStatus()

	c.responder.PublishSuccessStatusRespond(queueMessage.Type, queueMessage.MessageID, replyTo, status)
}

func (c *consumer) handleHandshakeMessage(queueMessage messages.QueueMessage, replyTo string) {
	languages := languages.GetSupportedLanguagesWithVersions()

	c.responder.PublishSuccessHandshakeRespond(queueMessage.Type, queueMessage.MessageID, replyTo, languages)
}
