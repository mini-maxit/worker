package pipeline

import (
	"errors"
	"fmt"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/rabbitmq/responder"
	"github.com/mini-maxit/worker/internal/stages/compiler"
	"github.com/mini-maxit/worker/internal/stages/executor"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/internal/stages/verifier"
	"github.com/mini-maxit/worker/pkg/constants"
	customErr "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"

	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type Worker interface {
	ProcessTask(messageID, responseQueue string, task *messages.TaskQueueMessage)
	GetState() WorkerState
	UpdateStatus(status constants.WorkerStatus)
	GetProcessingMessageID() string
	GetId() int
}

type WorkerState struct {
	Status              constants.WorkerStatus `json:"status"`
	ProcessingMessageID string                 `json:"processing_message_id"`
}

type worker struct {
	id            int
	state         WorkerState
	responseQueue string
	compiler      compiler.Compiler
	packager      packager.Packager
	executor      executor.Executor
	verifier      verifier.Verifier
	responder     responder.Responder
	logger        *zap.SugaredLogger
}

func NewWorker(
	id int,
	compiler compiler.Compiler,
	packager packager.Packager,
	executor executor.Executor,
	verifier verifier.Verifier,
	responder responder.Responder,
) Worker {
	logger := logger.NewNamedLogger(fmt.Sprintf("worker-%d", id))

	return &worker{
		id:        id,
		state:     WorkerState{Status: constants.WorkerStatusIdle, ProcessingMessageID: ""},
		compiler:  compiler,
		packager:  packager,
		executor:  executor,
		verifier:  verifier,
		responder: responder,
		logger:    logger,
	}
}

func (ws *worker) GetId() int {
	return ws.id
}

func (ws *worker) GetState() WorkerState {
	return ws.state
}

func (ws *worker) UpdateStatus(status constants.WorkerStatus) {
	ws.state.Status = status
}

func (ws *worker) GetProcessingMessageID() string {
	return ws.state.ProcessingMessageID
}

func (ws *worker) ProcessTask(messageID, responseQueue string, task *messages.TaskQueueMessage) {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				ws.responder.PublishTaskErrorToResponseQueue(
					constants.QueueMessageTypeTask,
					ws.state.ProcessingMessageID,
					ws.responseQueue,
					err,
				)
			} else {
				ws.logger.Errorf("Recovered value is not an error: %v", r)
			}
		}
	}()

	ws.logger.Infof("Processing task [MsgID: %s]", messageID)
	ws.state.ProcessingMessageID = messageID
	ws.responseQueue = responseQueue
	defer func() {
		ws.state.ProcessingMessageID = ""
		ws.responseQueue = ""
	}()

	langType, err := languages.ParseLanguageType(task.LanguageType)
	if err != nil {
		ws.logger.Errorf("Invalid language type %s: %s", task.LanguageType, err)
		ws.responder.PublishTaskErrorToResponseQueue(
			constants.QueueMessageTypeTask,
			ws.state.ProcessingMessageID,
			ws.responseQueue,
			err,
		)
		return
	}

	dc, err := ws.packager.PrepareSolutionPackage(task, langType, messageID)
	if err != nil {
		ws.responder.PublishTaskErrorToResponseQueue(
			constants.QueueMessageTypeTask,
			ws.state.ProcessingMessageID,
			ws.responseQueue,
			err,
		)
		return
	}

	defer func() {
		if err := utils.RemoveIO(dc.PackageDirPath, true, true); err != nil {
			ws.logger.Errorf("[MsgID %s] Failed to remove temp directory: %s", messageID, err)
		}
	}()

	err = ws.compiler.CompileSolutionIfNeeded(
		langType,
		task.LanguageVersion,
		dc.UserSolutionPath,
		dc.UserExecFilePath,
		dc.CompileErrFilePath,
		messageID)

	if err != nil {
		if errors.Is(err, customErr.ErrCompilationFailed) {
			ws.publishCompilationError(dc, task.TestCases)
			return
		}

		ws.responder.PublishTaskErrorToResponseQueue(
			constants.QueueMessageTypeTask,
			ws.state.ProcessingMessageID,
			ws.responseQueue,
			err,
		)
		return
	}

	limits := make([]solution.Limit, len(task.TestCases))
	for i, tc := range task.TestCases {
		limits[i] = solution.Limit{
			TimeMs:   tc.TimeLimitMs,
			MemoryKb: tc.MemoryLimitKB,
		}
	}

	cfg := executor.CommandConfig{
		MessageID:       messageID,
		DirConfig:       dc,
		LanguageType:    langType,
		LanguageVersion: task.LanguageVersion,
		TestCases:       task.TestCases,
	}

	err = ws.executor.ExecuteCommand(cfg)
	if err != nil {
		ws.responder.PublishTaskErrorToResponseQueue(
			constants.QueueMessageTypeTask,
			ws.state.ProcessingMessageID,
			ws.responseQueue,
			err,
		)
		return
	}

	solutionResult := ws.verifier.EvaluateAllTestCases(dc, task.TestCases, messageID, langType)

	err = ws.packager.SendSolutionPackage(dc, task.TestCases /*hasCompilationErr*/, false, messageID)
	if err != nil {
		ws.responder.PublishTaskErrorToResponseQueue(
			constants.QueueMessageTypeTask,
			ws.state.ProcessingMessageID,
			ws.responseQueue,
			err,
		)
		return
	}

	ws.responder.PublishPayloadTaskRespond(
		constants.QueueMessageTypeTask,
		ws.state.ProcessingMessageID,
		ws.responseQueue,
		solutionResult,
	)
	ws.logger.Infof("Finished processing task [MsgID: %s]", messageID)
}

func (ws *worker) publishCompilationError(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase) {
	sendErr := ws.packager.SendSolutionPackage(dirConfig, testCases, true, ws.state.ProcessingMessageID)
	if sendErr != nil {
		ws.responder.PublishTaskErrorToResponseQueue(
			constants.QueueMessageTypeTask,
			ws.state.ProcessingMessageID,
			ws.responseQueue,
			sendErr,
		)
		return
	}

	solutionResult := solution.Result{
		StatusCode: solution.CompilationError,
		Message:    constants.SolutionMessageCompilationError,
	}
	ws.responder.PublishPayloadTaskRespond(
		constants.QueueMessageTypeTask,
		ws.state.ProcessingMessageID,
		ws.responseQueue,
		solutionResult,
	)
}
