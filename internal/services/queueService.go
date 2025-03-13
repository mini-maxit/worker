package services

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/logger"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type QueueService interface {
	Listen()
}

type queueService struct {
	channel         *amqp.Channel
	mainQueueName   string
	workerQueueName string
	workerPool      *WorkerPool
	fileService     FileService
	runnerService   RunnerService
	logger          *zap.SugaredLogger
}

type QueueMessage struct {
	Type      string          `json:"type"`
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}

type StopQueueMessage struct {
	WorkerID int `json:"worker_id"`
}

func NewQueueService(mainChannel *amqp.Channel, workerQueueName, mainQueueName string, fileService FileService, runnerService RunnerService, workerPool *WorkerPool) QueueService {
	logger := logger.NewNamedLogger("queueService")

	return &queueService{
		channel:         mainChannel,
		mainQueueName:   mainQueueName,
		workerQueueName: workerQueueName,
		workerPool:      workerPool,
		fileService:     fileService,
		runnerService:   runnerService,
		logger:          logger,
	}
}

func (qs *queueService) Listen() {
	qs.logger.Infof("Declaring queues %s and %s", qs.mainQueueName, qs.workerQueueName)

	args := make(amqp.Table)
	args["x-max-priority"] = 2
	_, err := qs.channel.QueueDeclare(qs.mainQueueName, true, false, false, false, args)
	if err != nil {
		qs.logger.Panicf("Failed to declare queue %s: %s", qs.mainQueueName, err)
	}

	_, err = qs.channel.QueueDeclare(qs.workerQueueName, true, false, false, false, nil)
	if err != nil {
		qs.logger.Panicf("Failed to declare queue %s: %s", qs.workerQueueName, err)
	}

	qs.logger.Infof("Listening for messages on queue %s", qs.mainQueueName)

	err = qs.workerPool.StartWorker(qs.fileService, qs.runnerService)
	if err != nil {
		qs.logger.Panicf("Failed to start worker: %s", err)
	}

	msgs, err := qs.channel.Consume(qs.mainQueueName, "", true, false, false, false, nil)
	if err != nil {
		qs.logger.Panicf("Failed to consume messages from queue %s: %s", qs.mainQueueName, err)
	}

	for msg := range msgs {
		var queueMessage QueueMessage
		err := json.Unmarshal(msg.Body, &queueMessage)
		if err != nil {
			qs.logger.Errorf("Failed to unmarshal message: %s", err)
			continue
		}

		if queueMessage.Type == constants.QueueMessageTypeTask || queueMessage.Type == constants.QueueMessageTypeHandshake {
			qs.logger.Infof("Processing task message")
			var task TaskQueueMessage
			err := json.Unmarshal(queueMessage.Payload, &task)
			if err != nil {
				qs.logger.Errorf("Failed to unmarshal task message: %s", err)
				continue
			}

			err = qs.channel.Publish("", qs.workerQueueName, false, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        msg.Body,
				ReplyTo:     msg.ReplyTo,
			})
			if err != nil {
				qs.logger.Errorf("Failed to publish message to worker queue: %s", err)
			}
		} else if queueMessage.Type == constants.QueueMessageTypeStatus {
			qs.logger.Infof("Processing status message")
			status := qs.workerPool.GetWorkersStatus()
			payload, err := json.Marshal(status)
			if err != nil {
				qs.logger.Errorf("Failed to marshal status message: %s", err)
				continue
			}

			response := QueueMessage{
				Type:      constants.QueueMessageTypeStatus,
				MessageID: queueMessage.MessageID,
				Payload:   payload,
			}

			responseJSON, err := json.Marshal(response)
			if err != nil {
				qs.logger.Errorf("Failed to marshal status message: %s", err)
				continue
			}

			err = qs.channel.Publish("", msg.ReplyTo, false, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        responseJSON,
			})
			if err != nil {
				qs.logger.Errorf("Failed to publish status message: %s", err)
			}
		} else if queueMessage.Type == constants.QueueMessageTypeStop {
			qs.logger.Infof("Processing stop message")
			var stop StopQueueMessage
			err := json.Unmarshal(queueMessage.Payload, &stop)
			if err != nil {
				qs.logger.Errorf("Failed to unmarshal stop message: %s", err)
				continue
			}

			err = qs.workerPool.StopWorker(stop.WorkerID)
			if err != nil {
				qs.logger.Errorf("Failed to stop worker %d: %s", stop.WorkerID, err)
			}

		} else if queueMessage.Type == constants.QueueMessageTypeAdd {
			qs.logger.Infof("Processing add worker message")
			err := qs.workerPool.StartWorker(qs.fileService, qs.runnerService)
			if err != nil {
				qs.logger.Errorf("Failed to add worker: %s", err)
			}
		} else {
			qs.logger.Errorf("Unknown message type: %s", queueMessage.Type)
		}
	}

}
