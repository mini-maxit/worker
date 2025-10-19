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
	ProcessTask(messageID string, task *messages.TaskQueueMessage)
	GetStatus() string
	UpdateStatus(status string)
	GetProcessingMessageID() string
	GetId() int
}

type worker struct {
	id                  int
	status              string
	processingMessageID string
	compiler            compiler.Compiler
	packager            packager.Packager
	executor            executor.Executor
	verifier            verifier.Verifier
	responder           responder.Responder
	logger              *zap.SugaredLogger
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
		id:                  id,
		status:              constants.WorkerStatusIdle,
		processingMessageID: "",
		compiler:            compiler,
		packager:            packager,
		executor:            executor,
		verifier:            verifier,
		responder:           responder,
		logger:              logger,
	}
}

func (ws *worker) GetId() int {
	return ws.id
}

func (ws *worker) GetStatus() string {
	return ws.status
}

func (ws *worker) UpdateStatus(status string) {
	ws.status = status
}

func (ws *worker) GetProcessingMessageID() string {
	return ws.processingMessageID
}

func (ws *worker) ProcessTask(messageID string, task *messages.TaskQueueMessage) {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				ws.publishError(err)
			} else {
				ws.logger.Errorf("Recovered value is not an error: %v", r)
			}
		}
	}()

	ws.logger.Infof("Processing task [MsgID: %s]", messageID)
	ws.processingMessageID = messageID
	defer func() { ws.processingMessageID = "" }()

	// Parse language type
	langType, err := languages.ParseLanguageType(task.LanguageType)
	if err != nil {
		ws.publishError(err)
		return
	}

	// Download all files and set up directories
	dc, err := ws.packager.PrepareSolutionPackage(task, messageID)
	if err != nil {
		ws.publishError(err)
		return
	}

	defer func() {
		// Clean up temporary directories
		if err := utils.RemoveIO(dc.PackageDirPath, true, true); err != nil {
			ws.logger.Errorf("[MsgID %s] Failed to remove temp directory: %s", messageID, err)
		}
	}()

	// Compile solution if needed
	err = ws.compiler.CompileSolutionIfNeeded(
		langType,
		task.LanguageVersion,
		dc.UserSolutionPath,
		dc.UserExecFilePath,
		dc.CompileErrFilePath,
		messageID)

	if err != nil {
		if errors.Is(err, customErr.ErrCompilationFailed) {
			ws.publishCompilationError(err, dc, task.TestCases)
			return
		}

		ws.publishError(err)
		return
	}

	limits := make([]solution.Limit, len(task.TestCases))
	for i, tc := range task.TestCases {
		limits[i] = solution.Limit{
			TimeMs:   tc.TimeLimitMs,
			MemoryKb: tc.MemoryLimitKB,
		}
	}

	// Run solution
	cfg := executor.CommandConfig{
		MessageID:       messageID,
		DirConfig:       dc,
		LanguageType:    langType,
		LanguageVersion: task.LanguageVersion,
		TestCases:       task.TestCases,
	}
	err = ws.executor.ExecuteCommand(cfg)
	if err != nil {
		ws.publishError(err)
		return
	}

	// Evaluate solution
	solutionResult := ws.verifier.EvaluateAllTestCases(dc, task.TestCases, messageID)

	// Store solution results
	err = ws.packager.SendSolutionPackage(dc, task.TestCases /*hasCompilationErr*/, false)
	if err != nil {
		ws.publishError(err)
		return
	}

	// Send response message
	ws.publishPayload(solutionResult)
}

func (ws *worker) publishPayload(solutionResult solution.Result) {
	ws.logger.Infof("Publishing payload response [MsgID: %s]", ws.processingMessageID)
	publishErr := ws.responder.PublishPayloadTaskRespond(
		constants.QueueMessageTypeTask,
		ws.processingMessageID,
		solutionResult)

	if publishErr != nil {
		ws.logger.Errorf("Failed to publish success response: %s", publishErr)
		ws.responder.PublishErrorToResponseQueue(constants.QueueMessageTypeTask, ws.processingMessageID, publishErr)
	}
}

func (ws *worker) publishError(err error) {
	ws.logger.Errorf("Error: %s", err)
	ws.responder.PublishErrorToResponseQueue(constants.QueueMessageTypeTask, ws.processingMessageID, err)
}

func (ws *worker) publishCompilationError(err error, dirConfig *packager.TaskDirConfig, testCases []messages.TestCase) {
	ws.logger.Errorf("Compilation error: %s", err)
	sendErr := ws.packager.SendSolutionPackage(dirConfig, testCases, true)
	if sendErr != nil {
		ws.logger.Errorf("Failed to send solution package: %s", sendErr)
	}

	solutionResult := solution.Result{
		StatusCode: solution.CompilationError,
		Message:    "Compilation failed",
	}
	respErr := ws.responder.PublishPayloadTaskRespond(
		constants.QueueMessageTypeTask,
		ws.processingMessageID, solutionResult,
	)
	if respErr != nil {
		ws.logger.Errorf("Failed to publish response: %s", respErr)
	}
}
