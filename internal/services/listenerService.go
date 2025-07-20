package services

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/languages"
	"github.com/mini-maxit/worker/internal/logger"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	e "errors"
)

type ListenerService interface {
	Listen(queueName string)
}

type listenerService struct {
	channel        *amqp.Channel
	workerPool     WorkerPoolService
	messageService MessageService
	logger         *zap.SugaredLogger
}

type QueueMessage struct {
	Type      string          `json:"type"`
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}

type ResponseQueueMessage struct {
	Type      string          `json:"type"`
	MessageID string          `json:"message_id"`
	Ok        bool            `json:"ok"`
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

func NewListenerService(mainChannel *amqp.Channel,
	workerPool WorkerPoolService,
	messageService MessageService,
) ListenerService {
	logger := logger.NewNamedLogger("listenerService")

	return &listenerService{
		channel:        mainChannel,
		workerPool:     workerPool,
		messageService: messageService,
		logger:         logger,
	}
}

func (ls *listenerService) Listen(queueName string) {
	ls.logger.Infof("Listening for messages on queue: %s", queueName)

	msgs, err := ls.channel.Consume(queueName, "", true, false, false, false, nil)
	if err != nil {
		ls.logger.Panicf("Failed to consume messages from queue %s: %s", queueName, err)
	}

	for msg := range msgs {
		var queueMessage QueueMessage
		err := json.Unmarshal(msg.Body, &queueMessage)
		if err != nil {
			ls.logger.Errorf("Failed to unmarshal message: %s", err)
			err = ls.messageService.PublishErrorToQueue(msg.ReplyTo, queueMessage.Type, queueMessage.MessageID, err)
			if err != nil {
				ls.logger.Errorf("Failed to publish error message: %s", err)
			}
			continue
		}

		switch queueMessage.Type {
		case constants.QueueMessageTypeTask:
			ls.logger.Infof("Received task message: %s", queueMessage.MessageID)
			ls.handleTaskMessage(queueMessage, msg.ReplyTo)
		case constants.QueueMessageTypeStatus:
			ls.logger.Infof("Received status message: %s", queueMessage.MessageID)
			ls.handleStatusMessage(queueMessage, msg.ReplyTo)
		case constants.QueueMessageTypeHandshake:
			ls.logger.Infof("Received handshake message: %s", queueMessage.MessageID)
			ls.handleHandshakeMessage(queueMessage, msg.ReplyTo)
		default:
			ls.logger.Errorf("Unknown message type: %s", queueMessage.Type)
			err = ls.messageService.PublishErrorToQueue(
				msg.ReplyTo,
				queueMessage.MessageID,
				queueMessage.Type,
				errors.ErrUnknownMessageType)
			if err != nil {
				ls.logger.Errorf("Failed to publish error message: %s", err)
			}
			continue
		}
	}
}

func (ls *listenerService) handleTaskMessage(queueMessage QueueMessage, replyTo string) {
	ls.logger.Infof("Processing task message")

	var task TaskQueueMessage
	if err := json.Unmarshal(queueMessage.Payload, &task); err != nil {
		ls.logger.Errorf("Failed to unmarshal task message: %s", err)
		pubErr := ls.messageService.PublishErrorToQueue(
			replyTo,
			queueMessage.MessageID,
			queueMessage.Type,
			err)
		if pubErr != nil {
			ls.logger.Errorf("Failed to publish error message: %s", pubErr)
		}
		return
	}

	err := ls.workerPool.ProcessTask(replyTo, queueMessage.MessageID, task)
	if err == nil {
		return
	}

	ls.logger.Errorf("Failed to process task message: %s", err)

	if e.Is(err, errors.ErrFailedToGetFreeWorker) {
		requeueErr := ls.messageService.RequeueTaskWithPriority2(replyTo, queueMessage)
		if requeueErr != nil {
			ls.logger.Errorf("Failed to requeue task with higher priority: %s", requeueErr)
		}
		return
	}

	pubErr := ls.messageService.PublishErrorToQueue(
		replyTo,
		queueMessage.MessageID,
		queueMessage.Type,
		err)

	if pubErr != nil {
		ls.logger.Errorf("Failed to publish error message: %s", pubErr)
	}
}

func (ls *listenerService) handleStatusMessage(queueMessage QueueMessage, replyTo string) {
	ls.logger.Infof("Processing status message")
	status := ls.workerPool.GetWorkersStatus()
	payload, err := json.Marshal(status)
	if err != nil {
		ls.logger.Errorf("Failed to marshal status message: %s", err)
		err = ls.messageService.PublishErrorToQueue(replyTo, queueMessage.MessageID, queueMessage.Type, err)
		if err != nil {
			ls.logger.Errorf("Failed to publish error message: %s", err)
		}
	}

	err = ls.messageService.PublishSuccessToQueue(replyTo, queueMessage.MessageID, queueMessage.Type, payload)
	if err != nil {
		ls.logger.Errorf("Failed to publish status message: %s", err)
		err = ls.messageService.PublishErrorToQueue(replyTo, queueMessage.MessageID, queueMessage.Type, err)
		if err != nil {
			ls.logger.Errorf("Failed to publish error message: %s", err)
		}
	}
}

func (ls *listenerService) handleHandshakeMessage(queueMessage QueueMessage, replyTo string) {
	ls.logger.Infof("Processing handshake message")

	supportedLanguages := languages.GetSupportedLanguagesWithVersions()

	var languagesList []map[string]interface{}
	for name, versions := range supportedLanguages {
		languagesList = append(languagesList, map[string]interface{}{
			"name":     name,
			"versions": versions,
		})
	}

	payload, err := json.Marshal(map[string]interface{}{
		"languages": languagesList,
	})
	if err != nil {
		ls.logger.Errorf("Failed to marshal supported languages: %s", err)
		err = ls.messageService.PublishErrorToQueue(replyTo, queueMessage.MessageID, queueMessage.Type, err)
		if err != nil {
			ls.logger.Errorf("Failed to publish error message: %s", err)
		}
	}

	err = ls.messageService.PublishSuccessToQueue(replyTo, queueMessage.MessageID, queueMessage.Type, payload)
	if err != nil {
		ls.logger.Errorf("Failed to publish supported languages: %s", err)
		err = ls.messageService.PublishErrorToQueue(replyTo, queueMessage.MessageID, queueMessage.Type, err)
		if err != nil {
			ls.logger.Errorf("Failed to publish error message: %s", err)
		}
	}
}
