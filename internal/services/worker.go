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
	userOutputDirName   string
	timeLimits          []int
	memoryLimits        []int
	chrootDirPath       string
	useChroot           bool
}

func (ws *Worker) ProcessTask(responseQueueName string, messageID string, task TaskQueueMessage) {
	defer func() {
		if r := recover(); r != nil {
			ws.logger.Errorf("Worker panicked: %v", r)
			if err, ok := r.(error); ok {
				ws.publishError(responseQueueName, messageID, err)
			} else {
				ws.logger.Errorf("Recovered value is not an error: %v", r)
			}
		}
	}()

	ws.logger.Infof("Processing task [MsgID: %s]", messageID)
	ws.processingMessageID = messageID
	defer func() { ws.processingMessageID = "" }()

	dc, solutionFileName, langType, err := ws.prepareTaskEnvironment(task)
	if err != nil {
		ws.publishError(responseQueueName, messageID, err)
		return
	}
	defer func() {
		if err := utils.RemoveIO(dc.UserSolutionDirPath, true, true); err != nil {
			ws.logger.Errorf("[MsgID %s] Failed to remove temp directory: %s", messageID, err)
		}
	}()

	chrootPath := task.ChrootDirPath
	if chrootPath == "" {
		chrootPath = constants.BaseChrootDir
	}
	useChroot := task.UseChroot != "false"

	taskForRunner := &TaskForRunner{
		taskFilesDirPath:    dc.TaskFilesDirPath,
		userSolutionDirPath: dc.UserSolutionDirPath,
		languageType:        langType,
		languageVersion:     task.LanguageVersion,
		solutionFileName:    solutionFileName,
		inputDirName:        constants.InputDirName,
		outputDirName:       constants.OutputDirName,
		userOutputDirName:   constants.UserOutputDirName,
		timeLimits:          task.TimeLimits,
		memoryLimits:        task.MemoryLimits,
		chrootDirPath:       chrootPath,
		useChroot:           useChroot,
	}

	solutionResult := ws.runnerService.RunSolution(taskForRunner, messageID)
	ws.storeAndPublishSolutionResult(
		solutionResult,
		*dc,
		task,
		messageID,
		responseQueueName)
}

func (ws *Worker) prepareTaskEnvironment(task TaskQueueMessage,
) (*TaskDirConfig, string, languages.LanguageType, error) {
	dc, err := ws.fileService.HandleTaskPackage(task.TaskID, task.UserID, task.SubmissionNumber)
	if err != nil {
		return nil, "", 0, err
	}

	langType, err := languages.ParseLanguageType(task.LanguageType)
	if err != nil {
		return nil, "", 0, err
	}

	solutionFileName, err := languages.GetSolutionFileNameWithExtension(constants.SolutionFileBaseName, langType)
	if err != nil {
		return nil, "", 0, err
	}

	return dc, solutionFileName, langType, nil
}

func (ws *Worker) publishError(queue, messageID string, err error) {
	ws.logger.Errorf("Error: %s", err)
	publishErr := PublishErrorToResponseQueue(ws.channel, queue, constants.QueueMessageTypeTask, messageID, err)
	if publishErr != nil {
		ws.logger.Errorf("Failed to publish error to response queue: %s", publishErr)
	}
}

func (ws *Worker) storeAndPublishSolutionResult(
	solutionResult solution.Result,
	dc TaskDirConfig,
	task TaskQueueMessage,
	messageID, responseQueueName string,
) {
	if solutionResult.StatusCode != solution.InitializationError {
		ws.logger.Infof("Storing solution result [MsgID: %s]", messageID)
		err := ws.fileService.StoreSolutionResult(
			solutionResult,
			dc.TaskFilesDirPath,
			task.UserID,
			task.TaskID,
			task.SubmissionNumber,
		)
		if err != nil {
			ws.publishError(responseQueueName, messageID, err)
			return
		}
	} else {
		ws.logger.Infof("Initialization error occurred. Skipping storing solution result [MsgID: %s]", messageID)
	}

	payload, err := json.Marshal(solutionResult)
	if err != nil {
		ws.publishError(responseQueueName, messageID, err)
		return
	}

	ws.logger.Infof("Publishing solution result [MsgID: %s]", messageID)
	err = PublishSucessToResponseQueue(ws.channel, responseQueueName, constants.QueueMessageTypeTask, messageID, payload)
	if err != nil {
		ws.publishError(responseQueueName, messageID, err)
	}
}
