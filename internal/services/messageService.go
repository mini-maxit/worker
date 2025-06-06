package services

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/logger"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type MessageService interface {
	DeclareQueue(queueName string, queuePriority int) error
	PublishErrorToQueue(responseQueueName, messageID, messageType string, err error) error
	PublishSuccessToQueue(responseQueueName, messageID, messageType string, payload []byte) error
	RequeueTaskWithPriority2(queueName string, queueMessage QueueMessage) error
}

type messageService struct {
	channel *amqp.Channel
	logger  *zap.SugaredLogger
}

func NewMessageService(channel *amqp.Channel) MessageService {
	logger := logger.NewNamedLogger("messageService")

	return &messageService{
		channel: channel,
		logger:  logger,
	}
}

func (ms *messageService) DeclareQueue(queueName string, queuePriority int) error {
	ms.logger.Infof("Declaring queue: %s", queueName)
	args := make(amqp.Table)
	args["x-max-priority"] = queuePriority
	_, err := ms.channel.QueueDeclare(queueName, true, false, false, false, args)
	if err != nil {
		ms.logger.Panicf("Failed to declare queue %s: %s", queueName, err)
	}
	return nil
}

func (ms *messageService) PublishErrorToQueue(
	responseQueueName, messageType, messageID string,
	err error,
) error {
	ms.logger.Errorf("Publishing error to response queue [MsgID: %s]: %s", messageID, err.Error())
	errorPayload := map[string]string{"error": err.Error()}
	payload, jsonErr := json.Marshal(errorPayload)
	if jsonErr != nil {
		ms.logger.Errorf("Failed to marshal error payload: %s", jsonErr.Error())
		return jsonErr
	}

	queueMessage := ResponseQueueMessage{
		Type:      messageType,
		MessageID: messageID,
		Ok:        false,
		Payload:   payload,
	}

	responseJSON, jsonErr := json.Marshal(queueMessage)
	if jsonErr != nil {
		ms.logger.Errorf("Failed to marshal response message: %s", jsonErr.Error())
		return jsonErr
	}

	err = ms.channel.Publish("", responseQueueName, false, false, amqp.Publishing{
		ContentType:   "application/json",
		CorrelationId: messageID,
		Body:          responseJSON,
	})

	if err != nil {
		ms.logger.Errorf("Failed to publish error message: %s", err.Error())
		return err
	}

	ms.logger.Infof("Published error message to response queue [MsgID: %s]", messageID)
	return nil
}

func (ms *messageService) PublishSuccessToQueue(
	responseQueueName,
	messageID, messageType string, payload []byte,
) error {
	ms.logger.Infof("Publishing success to response queue [MsgID: %s]", messageID)
	queueMessage := ResponseQueueMessage{
		Type:      messageType,
		MessageID: messageID,
		Ok:        true,
		Payload:   payload,
	}

	responseJSON, jsonErr := json.Marshal(queueMessage)
	if jsonErr != nil {
		ms.logger.Errorf("Failed to marshal response message: %s", jsonErr.Error())
		return jsonErr
	}

	err := ms.channel.Publish("", responseQueueName, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        responseJSON,
	})

	if err != nil {
		ms.logger.Errorf("Failed to publish success message: %s", err.Error())
		return err
	}

	ms.logger.Infof("Published success message to response queue [MsgID: %s]", messageID)
	return nil
}

func (ms *messageService) RequeueTaskWithPriority2(queueName string, queueMessage QueueMessage) error {
	priority := 2

	queueMessageJSON, err := json.Marshal(queueMessage)
	if err != nil {
		ms.logger.Errorf("Failed to marshal queue message: %s", err)
		return err
	}

	err = ms.channel.Publish("", queueName, false, false, amqp.Publishing{
		ContentType:   "application/json",
		CorrelationId: queueMessage.MessageID,
		Body:          queueMessageJSON,
		Priority:      uint8(priority),
	})

	if err != nil {
		ms.logger.Errorf("Failed to requeue task with higher priority: %s", err)
		return err
	}

	return nil
}
