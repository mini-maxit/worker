package services

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/logger"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type QueueService interface {
	DeclareQueue(queueName string) error
	PublishErrorToQueue(responseQueueName, messageID, messageType string, err error) error
	PublishSuccessToQueue(responseQueueName, messageID, messageType string, payload []byte) error
	RequeueTaskWithPriority2(queueName string, queueMessage QueueMessage) error
}

type queueService struct {
	channel *amqp.Channel
	logger  *zap.SugaredLogger
}

func NewQueueService(channel *amqp.Channel) QueueService {
	logger := logger.NewNamedLogger("queueService")

	return &queueService{
		channel: channel,
		logger:  logger,
	}
}

func (qs *queueService) DeclareQueue(queueName string) error {
	qs.logger.Infof("Declaring queue: %s", queueName)
	args := make(amqp.Table)
	args["x-max-priority"] = 3
	_, err := qs.channel.QueueDeclare(queueName, true, false, false, false, args)
	if err != nil {
		qs.logger.Panicf("Failed to declare queue %s: %s", queueName, err)
	}
	return nil
}

func (qs *queueService) PublishErrorToQueue(
	responseQueueName, messageType, messageID string,
	err error,
) error {
	qs.logger.Errorf("Publishing error to response queue [MsgID: %s]: %s", messageID, err.Error())
	errorPayload := map[string]string{"error": err.Error()}
	payload, jsonErr := json.Marshal(errorPayload)
	if jsonErr != nil {
		qs.logger.Errorf("Failed to marshal error payload: %s", jsonErr.Error())
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
		qs.logger.Errorf("Failed to marshal response message: %s", jsonErr.Error())
		return jsonErr
	}

	err = qs.channel.Publish("", responseQueueName, false, false, amqp.Publishing{
		ContentType:   "application/json",
		CorrelationId: messageID,
		Body:          responseJSON,
	})

	if err != nil {
		qs.logger.Errorf("Failed to publish error message: %s", err.Error())
		return err
	}

	qs.logger.Infof("Published error message to response queue [MsgID: %s]", messageID)
	return nil
}

func (qs *queueService) PublishSuccessToQueue(
	responseQueueName,
	messageID, messageType string, payload []byte,
) error {
	qs.logger.Infof("Publishing success to response queue [MsgID: %s]", messageID)
	queueMessage := ResponseQueueMessage{
		Type:      messageType,
		MessageID: messageID,
		Ok:        true,
		Payload:   payload,
	}

	responseJSON, jsonErr := json.Marshal(queueMessage)
	if jsonErr != nil {
		qs.logger.Errorf("Failed to marshal response message: %s", jsonErr.Error())
		return jsonErr
	}

	err := qs.channel.Publish("", responseQueueName, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        responseJSON,
	})

	if err != nil {
		qs.logger.Errorf("Failed to publish success message: %s", err.Error())
		return err
	}

	qs.logger.Infof("Published success message to response queue [MsgID: %s]", messageID)
	return nil
}

func (qs *queueService) RequeueTaskWithPriority2(queueName string, queueMessage QueueMessage) error {
	priority := 2

	queueMessageJSON, err := json.Marshal(queueMessage)
	if err != nil {
		qs.logger.Errorf("Failed to marshal queue message: %s", err)
		return err
	}

	err = qs.channel.Publish("", queueName, false, false, amqp.Publishing{
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
