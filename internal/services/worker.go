package services

import (
	"encoding/json"
	"fmt"

	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/languages"
	"github.com/mini-maxit/worker/internal/solution"
	"github.com/mini-maxit/worker/utils"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Worker struct {
	id            int
	status        string
	stopChan      chan bool
	fileService   FileService
	runnerService RunnerService
	logger        *zap.SugaredLogger
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

type TaskForRunner struct {
	taskFilesDirPath    string
	userSolutionDirPath string
	languageType        languages.LanguageType
	languageVersion     string
	solutionFileName    string
	inputDirName        string
	outputDirName       string
	timeLimits          []int
	memoryLimits        []int
	chrootDirPath       string
	useChroot           bool
}

func (ws *Worker) ProcessTask(task TaskQueueMessage, messageID string) (QueueMessage, error) {
	ws.logger.Infof("Processing task [MsgID: %s]", messageID)

	directoryConfig, err := ws.fileService.HandleTaskPackage(task.TaskID, task.UserID, task.SubmissionNumber)
	if err != nil {
		ws.logger.Errorf("Failed to handle task package: %s", err)
		return QueueMessage{}, errors.ErrFailedToHandleTaskPackage
	}

	defer func() {
		err := utils.RemoveIO(directoryConfig.UserSolutionDirPath, true, true)
		if err != nil {
			ws.logger.Errorf("[MsgID %s] Failed to remove temp directory: %s", messageID, err)
		}
	}()

	languageType, err := languages.ParseLanguageType(task.LanguageType)
	if err != nil {
		ws.logger.Errorf("[MsgID %s]Failed to parse language type: %s", messageID, err)
		return QueueMessage{}, errors.ErrFailedToParseLanguageType
	}

	solutionFileName, err := languages.GetSolutionFileNameWithExtension(constants.SolutionFileBaseName, languageType)
	if err != nil {
		ws.logger.Errorf("[MsgID %s] Failed to get solution file name: %s", messageID, err)
		return QueueMessage{}, errors.ErrFailedToGetSolutionFileName
	}

	if task.ChrootDirPath == "" {
		task.ChrootDirPath = constants.BaseChrootDir
	}

	var useChroot bool
	if task.UseChroot == "" {
		useChroot = true
	} else if task.UseChroot == "true" {
		useChroot = true
	} else {
		useChroot = false
	}

	taskForRunner := &TaskForRunner{
		taskFilesDirPath:    directoryConfig.TaskFilesDirPath,
		userSolutionDirPath: directoryConfig.UserSolutionDirPath,
		languageType:        languageType,
		languageVersion:     task.LanguageVersion,
		solutionFileName:    solutionFileName,
		inputDirName:        constants.InputDirName,
		outputDirName:       constants.OutputDirName,
		timeLimits:          task.TimeLimits,
		memoryLimits:        task.MemoryLimits,
		chrootDirPath:       task.ChrootDirPath,
		useChroot:           useChroot,
	}

	solutionResult := ws.runnerService.RunSolution(taskForRunner, messageID)
	if solutionResult.StatusCode != solution.InitializationError {
		ws.logger.Infof("Storing solution result [MsgID: %s]", messageID)
		err = ws.fileService.StoreSolutionResult(solutionResult, taskForRunner.taskFilesDirPath, task.UserID, task.TaskID, task.SubmissionNumber)
		if err != nil {
			return QueueMessage{}, err
		}
	} else {
		ws.logger.Infof("Initialization error occurred. Skipping storing solution result [MsgID: %s]", messageID)
	}

	payload, err := json.Marshal(solutionResult)
	if err != nil {
		ws.logger.Errorf("Failed to marshal solution result: %s", err)
		return QueueMessage{}, errors.ErrFailedToStoreSolution
	}

	return QueueMessage{
		Type:      constants.QueueMessageTypeTask,
		MessageID: messageID,
		Payload:   payload,
	}, nil
}

func (ws *Worker) ProcessHandshake(messageID string) (QueueMessage, error) {
	ws.logger.Infof("Processing handshake message")

	supportedLanguages := languages.GetSupportedLanguagesWithVersions()
	payload, err := json.Marshal(supportedLanguages)
	if err != nil {
		ws.logger.Errorf("Failed to marshal supported languages: %s", err)
		return QueueMessage{}, errors.ErrFailedToStoreSolution
	}

	return QueueMessage{
		Type:      constants.QueueMessageTypeHandshake,
		MessageID: messageID,
		Payload:   payload,
	}, nil
}

func (ws *Worker) ProcessMessage(queueMessage QueueMessage) (QueueMessage, error) {
	var response QueueMessage
	var err error

	switch queueMessage.Type {
	case constants.QueueMessageTypeHandshake:
		ws.logger.Infof("Processing handshake message")
		response, err = ws.ProcessHandshake(queueMessage.MessageID)
		if err != nil {
			ws.logger.Errorf("Failed to process message: %s", err)
			return QueueMessage{}, err
		}
	case constants.QueueMessageTypeTask:
		ws.logger.Infof("Processing task message")
		var task TaskQueueMessage
		err = json.Unmarshal(queueMessage.Payload, &task)
		if err != nil {
			ws.logger.Errorf("Failed to unmarshal task message: %s", err)
			return QueueMessage{}, err
		}
		response, err = ws.ProcessTask(task, queueMessage.MessageID)
		if err != nil {
			ws.logger.Errorf("Failed to process message: %s", err)
			return QueueMessage{}, err
		}
	default:
		ws.logger.Errorf("Unknown message type: %s", queueMessage.Type)
		return QueueMessage{}, errors.ErrInvalidQueueMessageType
	}

	return response, nil

}

func (ws *Worker) Listen(channel *amqp.Channel, workerQueueName string) {
	msgs, err := channel.Consume(workerQueueName, "", true, false, false, false, nil)
	if err != nil {
		ws.logger.Panicf("Failed to consume messages from queue %s: %s", workerQueueName, err)
	}

	for {
		ws.status = constants.WorkerStatusIdle
		select {
		case <-ws.stopChan:
			ws.logger.Infof("Stopping worker %s", ws.id)
			return
		case msg := <-msgs:
			ws.logger.Infof("Received message")
			ws.status = constants.WorkerStatusBusy

			var queueMessage QueueMessage
			var response QueueMessage
			err := json.Unmarshal(msg.Body, &queueMessage)
			if err == nil {
				response, err = ws.ProcessMessage(queueMessage)
				if err != nil {
					ws.logger.Errorf("Failed to process message: %s", err)
					response = QueueMessage{
						Type:      queueMessage.Type,
						MessageID: queueMessage.MessageID,
						Payload:   []byte(fmt.Sprintf(`{"error": "%s"}`, err.Error())),
					}
				}
			} else {
				ws.logger.Errorf("Failed to unmarshal message: %s", err)
				response = QueueMessage{
					Type:      queueMessage.Type,
					MessageID: queueMessage.MessageID,
					Payload:   []byte(fmt.Sprintf(`{"error": "%s"}`, err.Error())),
				}
			}

			responseJSON, err := json.Marshal(response)
			if err != nil {
				ws.logger.Errorf("Failed to marshal response: %s", err)
				continue
			}

			ws.logger.Infof("Sending response to queue %s", msg.ReplyTo)

			err = channel.Publish("", msg.ReplyTo, false, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        responseJSON,
			})
			if err != nil {
				ws.logger.Errorf("Failed to send response: %s", err)
				continue
			}
		}
	}
}
