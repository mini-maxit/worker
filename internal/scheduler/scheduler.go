package scheduler

import (
	"fmt"
	"sync"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/pipeline"
	"github.com/mini-maxit/worker/internal/storage"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/messages"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Scheduler interface {
	GetWorkersStatus() map[string]interface{}
	ProcessTask(responseQueueName, messageID string, task messages.TaskQueueMessage) error
	GetSupportedLanguages() map[string][]string
}

type scheduler struct {
	mu               sync.Mutex
	busyWorkersCount int
	workers          map[int]pipeline.Worker
	workerChannel    *amqp.Channel
	workerQueue      string
	maxWorkers       int
	logger           *zap.SugaredLogger
}

func NewScheduler(
	workerChannel *amqp.Channel,
	workerQueue string,
	maxWorkers int,
	storage storage.FileService,
) Scheduler {
	workers := make(map[int]pipeline.Worker, maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		workers[i] = pipeline.NewWorker(i, workerChannel, storage, logger.NewNamedLogger(fmt.Sprintf("worker-%d", i)))
	}

	workerPoolLogger := logger.NewNamedLogger("workerPool")

	return &scheduler{
		mu:            sync.Mutex{},
		workers:       workers,
		workerChannel: workerChannel,
		workerQueue:   workerQueue,
		maxWorkers:    maxWorkers,
		logger:        workerPoolLogger,
	}
}

func (s *scheduler) GetWorkersStatus() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	statuses := make(map[int]string, len(s.workers))

	for id, worker := range s.workers {
		if worker.GetStatus() == constants.WorkerStatusBusy {
			statuses[id] = worker.GetStatus() + " Processing message: " + worker.GetProcessingMessageID()
			continue
		}
		statuses[id] = worker.GetStatus()
	}

	return map[string]interface{}{
		"busy_workers":  s.busyWorkersCount,
		"total_workers": s.maxWorkers,
		"worker_status": statuses,
	}
}

func (s *scheduler) getFreeWorker() (pipeline.Worker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, worker := range s.workers {
		if worker.GetStatus() == constants.WorkerStatusIdle {
			worker.UpdateStatus(constants.WorkerStatusBusy)
			s.busyWorkersCount++
			return worker, nil
		}
	}

	return nil, errors.ErrFailedToGetFreeWorker
}

func (s *scheduler) ProcessTask(responseQueueName, messageID string, task messages.TaskQueueMessage) error {
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

		w.ProcessTask(responseQueueName, messageID, task)
	}(worker)

	return nil
}

func (s *scheduler) markWorkerAsIdle(worker pipeline.Worker) {
	s.logger.Infof("Marking worker as idle [WorkerID: %d]", worker.GetId())
	s.mu.Lock()
	defer s.mu.Unlock()

	worker.UpdateStatus(constants.WorkerStatusIdle)
	s.busyWorkersCount--

	s.logger.Infof("Worker marked as idle [WorkerID: %d]", worker.GetId())
}

// TODO: implement
func (s *scheduler) GetSupportedLanguages() map[string][]string {
	languages := make(map[string][]string)
	return languages
}
