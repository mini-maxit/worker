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
	mu            sync.Mutex
	workers       map[int]*Worker
	workerChannel *amqp.Channel
	workerQueue   string
	nextWorkerID  int
	maxWorkers    int
	logger        *zap.SugaredLogger
}

func NewWorkerPool(workerChannel *amqp.Channel, workerQueue string, maxWorkers int) *WorkerPool {
	logger := logger.NewNamedLogger("workerPool")

	return &WorkerPool{
		workers:       make(map[int]*Worker),
		workerChannel: workerChannel,
		workerQueue:   workerQueue,
		maxWorkers:    maxWorkers,
		nextWorkerID:  1,
		logger:        logger,
	}
}

func (wp *WorkerPool) StartWorker(fileService FileService, runnerService RunnerService) error {
	if len(wp.workers) >= wp.maxWorkers {
		return errors.ErrMaxWorkersReached
	}

	wp.mu.Lock()
	id := wp.nextWorkerID
	wp.nextWorkerID++

	logger := logger.NewNamedLogger(fmt.Sprintf("worker-%d", id))
	worker := &Worker{
		id:            id,
		status:        constants.WorkerStatusIdle,
		stopChan:      make(chan bool),
		fileService:   fileService,
		runnerService: runnerService,
		logger:        logger,
	}
	wp.workers[id] = worker
	wp.mu.Unlock()

	wp.logger.Infof("Starting worker %d", id)

	go worker.Listen(wp.workerChannel, wp.workerQueue)

	return nil
}

func (wp *WorkerPool) StopWorker(workerID int) error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if worker, exists := wp.workers[workerID]; exists {
		close(worker.stopChan)
		delete(wp.workers, workerID)
		wp.logger.Infof("Worker %d stopped", workerID)
		return nil
	} else {
		wp.logger.Errorf("Worker %d not found", workerID)
		return errors.ErrWorkerNotFound
	}
}

func (wp *WorkerPool) GetWorkersStatus() map[int]string {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	status := make(map[int]string)
	for id, worker := range wp.workers {
		status[id] = worker.status
	}
	return status
}
