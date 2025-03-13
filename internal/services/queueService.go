package services

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/languages"
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
	workerPool      *WorkerPool
	logger          *zap.SugaredLogger
}

type QueueMessage struct {
	Type      string          `json:"type"`
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}

type TaskQueueMessage struct {
	TaskID           int64  `json:"task_id"`
	UserID           int64  `json:"user_id"`
	SubmissionNumber int64  `json:"submission_number"`
	LanguageType     string `json:"language_type"`
	LanguageVersion  string `json:"language_version"`
	TimeLimits       []int  `json:"time_limits"`
	MemoryLimits     []int  `json:"memory_limits"`
	ChrootDirPath    string `json:"chroot_dir_path,omitempty"` // Optional for test purposes
	UseChroot        string `json:"use_chroot,omitempty"`      // Optional for test purposes
}

func NewQueueService(mainChannel *amqp.Channel, workerQueueName string, workerPool *WorkerPool) QueueService {
	logger := logger.NewNamedLogger("queueService")

	return &queueService{
		channel:         mainChannel,
		workerQueueName: workerQueueName,
		workerPool:      workerPool,
		logger:          logger,
	}
}

func (qs *queueService) Listen() {
	qs.logger.Infof("Declaring queue %s", qs.workerQueueName)

	args := make(amqp.Table)
	args["x-max-priority"] = 3
	_, err := qs.channel.QueueDeclare(qs.workerQueueName, true, false, false, false, args)
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
			err = PublishErrorToResponseQueue(qs.channel, msg.ReplyTo, queueMessage.Type, queueMessage.MessageID, err)
			if err != nil {
				qs.logger.Errorf("Failed to publish error message: %s", err)
			}
			continue
		}

		if queueMessage.Type == constants.QueueMessageTypeTask {
			qs.logger.Infof("Processing task message")
			var task TaskQueueMessage
			err := json.Unmarshal(queueMessage.Payload, &task)
			if err != nil {
				qs.logger.Errorf("Failed to unmarshal task message: %s", err)
				err = PublishErrorToResponseQueue(qs.channel, msg.ReplyTo, queueMessage.Type, queueMessage.MessageID, err)
				if err != nil {
					qs.logger.Errorf("Failed to publish error message: %s", err)
				}
				continue
			}

			err = qs.workerPool.ProcessTask(msg.ReplyTo, queueMessage.MessageID, task)
			if err != nil {
				qs.logger.Errorf("Failed to process task message: %s", err)
				if err == errors.ErrFailedToGetFreeWorker {
					err = qs.RequeueTaskWithHigherPriority(queueMessage)
					if err != nil {
						qs.logger.Errorf("Failed to requeue task with higher priority: %s", err)
					}
				} else {
					err = PublishErrorToResponseQueue(qs.channel, msg.ReplyTo, queueMessage.Type, queueMessage.MessageID, err)
					if err != nil {
						qs.logger.Errorf("Failed to publish error message: %s", err)
					}
				}
				continue
			}

		} else if queueMessage.Type == constants.QueueMessageTypeStatus {
			qs.logger.Infof("Processing status message")
			status := qs.workerPool.GetWorkersStatus()
			payload, err := json.Marshal(status)
			if err != nil {
				qs.logger.Errorf("Failed to marshal status message: %s", err)
				err = PublishErrorToResponseQueue(qs.channel, msg.ReplyTo, queueMessage.Type, queueMessage.MessageID, err)
				if err != nil {
					qs.logger.Errorf("Failed to publish error message: %s", err)
				}
				continue
			}

			err = PublishSucessToResponseQueue(qs.channel, msg.ReplyTo, queueMessage.Type, queueMessage.MessageID, payload)
			if err != nil {
				qs.logger.Errorf("Failed to publish status message: %s", err)
				err = PublishErrorToResponseQueue(qs.channel, msg.ReplyTo, queueMessage.Type, queueMessage.MessageID, err)
				if err != nil {
					qs.logger.Errorf("Failed to publish error message: %s", err)
				}
			}

		} else if queueMessage.Type == constants.QueueMessageTypeHandshake {
			qs.logger.Infof("Processing handshake message")

			supportedLanguages := languages.GetSupportedLanguagesWithVersions()
			payload, err := json.Marshal(supportedLanguages)
			if err != nil {
				qs.logger.Errorf("Failed to marshal supported languages: %s", err)
				err = PublishErrorToResponseQueue(qs.channel, msg.ReplyTo, queueMessage.Type, queueMessage.MessageID, err)
				if err != nil {
					qs.logger.Errorf("Failed to publish error message: %s", err)
				}
				continue
			}

			err = PublishSucessToResponseQueue(qs.channel, msg.ReplyTo, queueMessage.Type, queueMessage.MessageID, payload)
			if err != nil {
				qs.logger.Errorf("Failed to publish supported languages: %s", err)
				err = PublishErrorToResponseQueue(qs.channel, msg.ReplyTo, queueMessage.Type, queueMessage.MessageID, err)
				if err != nil {
					qs.logger.Errorf("Failed to publish error message: %s", err)
				}
			}
		}
	}

}

func PublishErrorToResponseQueue(channel *amqp.Channel, responseQueueName, messageType, messageID string, err error) error {

	errorPayload := map[string]string{"error": err.Error()}
	payload, jsonErr := json.Marshal(errorPayload)
	if jsonErr != nil {
		return jsonErr
	}

	queueMessage := QueueMessage{
		Type:      messageType,
		MessageID: messageID,
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

func PublishSucessToResponseQueue(channel *amqp.Channel, responseQueueName, messageType, messageID string, payload []byte) error {
	logger := logger.NewNamedLogger("queueService")

	queueMessage := QueueMessage{
		Type:      messageType,
		MessageID: messageID,
		Payload:   payload,
	}

	logger.Infof("Marshalling queue message: %v", queueMessage)
	responseJSON, jsonErr := json.Marshal(queueMessage)
	if jsonErr != nil {
		return jsonErr
	}

	logger.Infof("Publishing response to queue: %s", responseQueueName)
	err := channel.Publish("", responseQueueName, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        responseJSON,
	})

	logger.Infof("Published response to queue: %s", responseQueueName)

	if err != nil {
		return err
	}

	return nil
}

func (qs *queueService) RequeueTaskWithHigherPriority(queueMessage QueueMessage) error {
	priority := 2

	queueMessageJSON, err := json.Marshal(queueMessage)
	if err != nil {
		qs.logger.Errorf("Failed to marshal queue message: %s", err)
		return err
	}

	err = qs.channel.Publish("", qs.workerQueueName, false, false, amqp.Publishing{
		ContentType:   "application/json",
		CorrelationId: queueMessage.MessageID,
		Body:          queueMessageJSON,
		Priority:      uint8(priority),
	})

	if err != nil {
		qs.logger.Errorf("Failed to requeue task with higher priority: %s", err)
		return err
	}

	return nil
}
