package scheduler

import (
	"sync"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/pipeline"
	"github.com/mini-maxit/worker/internal/rabbitmq/responder"
	"github.com/mini-maxit/worker/internal/stages/compiler"
	"github.com/mini-maxit/worker/internal/stages/executor"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/internal/stages/verifier"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/messages"
	"go.uber.org/zap"
)

type Scheduler interface {
	GetWorkersStatus() messages.ResponseWorkerStatusPayload
	ProcessTask(responseQueueName, messageID string, task *messages.TaskQueueMessage) error
}

type scheduler struct {
	mu         sync.Mutex
	workers    map[int]pipeline.Worker
	maxWorkers int
	logger     *zap.SugaredLogger
}

func NewScheduler(
	maxWorkers int,
	compiler compiler.Compiler,
	packager packager.Packager,
	executor executor.Executor,
	verifier verifier.Verifier,
	responder responder.Responder,

) Scheduler {
	return NewSchedulerWithWorkers(maxWorkers, nil, compiler, packager, executor, verifier, responder)
}

// NewSchedulerWithWorkers creates a Scheduler using the provided worker map.
// If `workers` is nil, it will create a worker map of size `maxWorkers`.
// This constructor is useful for tests where mock workers need to be injected.
func NewSchedulerWithWorkers(
	maxWorkers int,
	workers map[int]pipeline.Worker,
	compiler compiler.Compiler,
	packager packager.Packager,
	executor executor.Executor,
	verifier verifier.Verifier,
	responder responder.Responder,
) Scheduler {
	if workers == nil {
		workers = make(map[int]pipeline.Worker, maxWorkers)
		for i := range maxWorkers {
			workers[i] = pipeline.NewWorker(i, compiler, packager, executor, verifier, responder)
		}
	}

	workerPoolLogger := logger.NewNamedLogger("workerPool")

	return &scheduler{
		mu:         sync.Mutex{},
		workers:    workers,
		maxWorkers: maxWorkers,
		logger:     workerPoolLogger,
	}
}

func (s *scheduler) GetWorkersStatus() messages.ResponseWorkerStatusPayload {
	s.mu.Lock()
	defer s.mu.Unlock()

	states := make([]messages.WorkerStatus, 0, len(s.workers))
	busyWorkers := 0
	for _, worker := range s.workers {
		state := worker.GetState()
		states = append(states, messages.WorkerStatus{
			WorkerID:            worker.GetId(),
			Status:              state.Status,
			ProcessingMessageID: state.ProcessingMessageID,
		})
		if state.Status == constants.WorkerStatusBusy {
			busyWorkers++
		}
	}

	return messages.ResponseWorkerStatusPayload{
		BusyWorkers:  busyWorkers,
		TotalWorkers: s.maxWorkers,
		WorkerStatus: states,
	}
}

func (s *scheduler) getFreeWorker() (pipeline.Worker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, worker := range s.workers {
		if worker.GetState().Status == constants.WorkerStatusIdle {
			worker.UpdateStatus(constants.WorkerStatusBusy)
			return worker, nil
		}
	}

	return nil, errors.ErrFailedToGetFreeWorker
}

func (s *scheduler) ProcessTask(responseQueueName, messageID string, task *messages.TaskQueueMessage) error {
	s.logger.Infof("Processing task [MsgID: %s]", messageID)

	worker, err := s.getFreeWorker()
	if err != nil {
		s.logger.Errorf("No available workers: %s", err)
		return err
	}

	go func(w pipeline.Worker) {
		defer s.markWorkerAsIdle(w)
		defer func() {
			if r := recover(); r != nil {
				s.logger.Errorf("Worker panicked: %v", r)
			}
		}()

		w.ProcessTask(messageID, responseQueueName, task)
	}(worker)

	return nil
}

func (s *scheduler) markWorkerAsIdle(worker pipeline.Worker) {
	s.logger.Infof("Marking worker as idle [WorkerID: %d]", worker.GetId())
	s.mu.Lock()
	defer s.mu.Unlock()

	worker.UpdateStatus(constants.WorkerStatusIdle)

	s.logger.Infof("Worker marked as idle [WorkerID: %d]", worker.GetId())
}
