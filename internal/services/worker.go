package services

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/languages"
	"github.com/mini-maxit/worker/internal/solution"
	"github.com/mini-maxit/worker/utils"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Worker struct {
	id                  int
	status              string
	processingMessageID string
	channel             *amqp.Channel
	fileService         FileService
	runnerService       RunnerService
	logger              *zap.SugaredLogger
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

func (ws *Worker) ProcessTask(responseQueueName string, messageID string, task TaskQueueMessage) {
	defer func() {
		if r := recover(); r != nil {
			ws.logger.Errorf("Worker panicked: %v", r)
			err := PublishErrorToResponseQueue(ws.channel, responseQueueName, constants.QueueMessageTypeTask, messageID, r.(error))
			if err != nil {
				ws.logger.Errorf("Failed to publish error to response queue: %s", err)
			}
		}
	}()

	ws.logger.Infof("Processing task [MsgID: %s]", messageID)
	ws.processingMessageID = messageID
	defer func() {
		ws.processingMessageID = ""
	}()

	directoryConfig, err := ws.fileService.HandleTaskPackage(task.TaskID, task.UserID, task.SubmissionNumber)
	if err != nil {
		ws.logger.Errorf("Failed to handle task package: %s", err)
		err = PublishErrorToResponseQueue(ws.channel, responseQueueName, constants.QueueMessageTypeTask, messageID, err)
		if err != nil {
			ws.logger.Errorf("Failed to publish error to response queue: %s", err)
		}
		return
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
		err = PublishErrorToResponseQueue(ws.channel, responseQueueName, constants.QueueMessageTypeTask, messageID, err)
		if err != nil {
			ws.logger.Errorf("Failed to publish error to response queue: %s", err)
		}
		return
	}

	solutionFileName, err := languages.GetSolutionFileNameWithExtension(constants.SolutionFileBaseName, languageType)
	if err != nil {
		ws.logger.Errorf("[MsgID %s] Failed to get solution file name: %s", messageID, err)
		err = PublishErrorToResponseQueue(ws.channel, responseQueueName, constants.QueueMessageTypeTask, messageID, err)
		if err != nil {
			ws.logger.Errorf("Failed to publish error to response queue: %s", err)
		}
		return
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
			ws.logger.Errorf("Failed to store solution result: %s", err)
			err = PublishErrorToResponseQueue(ws.channel, responseQueueName, constants.QueueMessageTypeTask, messageID, err)
			if err != nil {
				ws.logger.Errorf("Failed to publish error to response queue: %s", err)
			}
			return
		}
	} else {
		ws.logger.Infof("Initialization error occurred. Skipping storing solution result [MsgID: %s]", messageID)
	}

	payload, err := json.Marshal(solutionResult)
	if err != nil {
		ws.logger.Errorf("Failed to marshal solution result: %s", err)
		err = PublishErrorToResponseQueue(ws.channel, responseQueueName, constants.QueueMessageTypeTask, messageID, err)
		if err != nil {
			ws.logger.Errorf("Failed to publish error to response queue: %s", err)
		}
		return
	}

	ws.logger.Infof("Publishing solution result [MsgID: %s]", messageID)
	err = PublishSucessToResponseQueue(ws.channel, responseQueueName, constants.QueueMessageTypeTask, messageID, payload)
	ws.logger.Infof("Published solution result [MsgID: %s]", messageID)
	if err != nil {
		ws.logger.Errorf("Failed to publish success to response queue: %s", err)
		err = PublishErrorToResponseQueue(ws.channel, responseQueueName, constants.QueueMessageTypeTask, messageID, err)
		if err != nil {
			ws.logger.Errorf("Failed to publish error to response queue: %s", err)
		}
	}
}
