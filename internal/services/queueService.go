package services

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/logger"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type QueueService interface {
	Listen()
}

type queueService struct {
	channel         *amqp.Channel
	workerQueueName string
	Worker          *Worker
	logger          *zap.SugaredLogger
}

type QueueMessage struct {
	Type      string          `json:"type"`
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}

func NewQueueService(channel *amqp.Channel, workerQueueName string, Worker *Worker) QueueService {
	logger := logger.NewNamedLogger("queueService")

	return &queueService{
		channel:         channel,
		Worker:          Worker,
		workerQueueName: workerQueueName,
		logger:          logger,
	}
}

func (qs *queueService) Listen() {
	qs.logger.Infof("Declaring queues %s", qs.workerQueueName)

	_, err := qs.channel.QueueDeclare(qs.workerQueueName, true, false, false, false, nil)
	if err != nil {
		qs.logger.Panicf("Failed to declare queue %s: %s", qs.workerQueueName, err)
	}

	qs.logger.Infof("Listening for messages on queue %s", qs.workerQueueName)

	msgs, err := qs.channel.Consume(qs.workerQueueName, "", true, false, false, false, nil)
	if err != nil {
		qs.logger.Panicf("Failed to consume messages from queue %s: %s", qs.workerQueueName, err)
	}

	for msg := range msgs {
		var queueMessage QueueMessage
		err := json.Unmarshal(msg.Body, &queueMessage)
		if err != nil {
			qs.logger.Errorf("Failed to unmarshal message: %s", err)
			continue
		}

		response, err := qs.Worker.ProcessMessage(queueMessage)
		if err != nil {
			qs.logger.Errorf("Failed to process message: %s", err)
			continue
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			qs.logger.Errorf("Failed to marshal response: %s", err)
			continue
		}

		qs.logger.Infof("Sending response: %s", response)

		err = qs.channel.Publish("", msg.ReplyTo, false, false, amqp.Publishing{
			ContentType: "application/json",
			Body:        responseJSON,
		})
		if err != nil {
			qs.logger.Errorf("Failed to publish response: %s", err)
		}

		qs.logger.Infof("Response sent")
	}
}
