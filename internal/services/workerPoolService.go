package services

import (
	"fmt"
	"sync"

	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/logger"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type WorkerPool struct {
	mu               sync.Mutex
	busyWorkersCount int
	workers          map[int]*Worker
	workerChannel    *amqp.Channel
	workerQueue      string
	maxWorkers       int
	logger           *zap.SugaredLogger
}

func NewWorkerPool(workerChannel *amqp.Channel, workerQueue string, maxWorkers int, fileService FileService, runnerService RunnerService) *WorkerPool {
	workers := make(map[int]*Worker, maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		workers[i] = &Worker{
			id:            i,
			status:        constants.WorkerStatusIdle,
			channel:       workerChannel,
			fileService:   fileService,
			runnerService: runnerService,
			logger:        logger.NewNamedLogger(fmt.Sprintf("worker-%d", i)),
		}
	}

	workerPoolLogger := logger.NewNamedLogger("workerPool")

	return &WorkerPool{
		mu:            sync.Mutex{},
		workers:       workers,
		workerChannel: workerChannel,
		workerQueue:   workerQueue,
		maxWorkers:    maxWorkers,
		logger:        workerPoolLogger,
	}
}

func (wp *WorkerPool) GetWorkersStatus() map[string]interface{} {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	statuses := make(map[int]string, len(wp.workers))

	for id, worker := range wp.workers {
		statuses[id] = worker.status
	}

	return map[string]interface{}{
		"busy_workers":  wp.busyWorkersCount,
		"worker_status": statuses,
	}
}

func (wp *WorkerPool) getFreeWorker() (*Worker, error) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	for _, worker := range wp.workers {
		if worker.status == constants.WorkerStatusIdle {
			worker.status = constants.WorkerStatusBusy
			wp.busyWorkersCount++
			return worker, nil
		}
	}

	return nil, errors.ErrFailedToGetFreeWorker
}

func (wp *WorkerPool) ProcessTask(responseQueueName, messageID string, task TaskQueueMessage) error {
	wp.logger.Infof("Processing task [MsgID: %s]", messageID)

	worker, err := wp.getFreeWorker()
	if err != nil {
		wp.logger.Errorf("No available workers: %s", err)
		return err
	}

	go func(w *Worker) {
		defer wp.markWorkerAsIdle(w)
		defer func() {
			if r := recover(); r != nil {
				wp.logger.Errorf("Worker panicked: %v", r)
			}
		}()

		w.ProcessTask(responseQueueName, messageID, task)
	}(worker)

	return nil
}

func (wp *WorkerPool) markWorkerAsIdle(worker *Worker) {
	wp.logger.Infof("Marking worker as idle [WorkerID: %d]", worker.id)
	wp.mu.Lock()
	defer wp.mu.Unlock()

	worker.status = constants.WorkerStatusIdle
	wp.busyWorkersCount--

	wp.logger.Infof("Worker marked as idle [WorkerID: %d]", worker.id)
}
