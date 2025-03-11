package services

import (
	"encoding/json"

	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/languages"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/solution"
	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type Worker struct {
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

func NewWorker(fileService FileService, runnerService RunnerService) *Worker {
	logger := logger.NewNamedLogger("Worker")

	return &Worker{
		fileService:   fileService,
		runnerService: runnerService,
		logger:        logger,
	}
}

func (ws *Worker) ProcessMessage(queueMessage QueueMessage) (QueueMessage, error) {
	ws.logger.Infof("Processing message %s", queueMessage.MessageID)

	switch queueMessage.Type {
	case constants.QueueMessageTypeTask:
		var task TaskQueueMessage
		err := json.Unmarshal(queueMessage.Payload, &task)
		if err != nil {
			ws.logger.Errorf("Failed to unmarshal task message: %s", err)
			return QueueMessage{}, errors.ErrFailedToUnmarshalTaskMessage
		}
		ws.logger.Infof("Processing task %d for user %d", task.TaskID, task.UserID)
		return ws.processTask(task, queueMessage.MessageID)
	case constants.QueueMessageTypeHandshake:
		ws.logger.Infof("Processing handshake message")
		return ws.processHandshake(queueMessage)
	default:
		ws.logger.Errorf("Unknown message type: %s", queueMessage.Type)
		return QueueMessage{}, errors.ErrInvalidQueueMessageType

	}
}

func (ws *Worker) processTask(task TaskQueueMessage, messageID string) (QueueMessage, error) {
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

func (ws *Worker) processHandshake(queueMessage QueueMessage) (QueueMessage, error) {
	ws.logger.Infof("Processing handshake message")

	supportedLanguages := languages.GetSupportedLanguagesWithVersions()
	payload, err := json.Marshal(supportedLanguages)
	if err != nil {
		ws.logger.Errorf("Failed to marshal supported languages: %s", err)
		return QueueMessage{}, errors.ErrFailedToStoreSolution
	}

	return QueueMessage{
		Type:      constants.QueueMessageTypeHandshake,
		MessageID: queueMessage.MessageID,
		Payload:   payload,
	}, nil
}
