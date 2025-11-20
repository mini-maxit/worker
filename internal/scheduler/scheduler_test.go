package scheduler_test

import (
	"errors"
	"testing"
	"time"

	gomock "go.uber.org/mock/gomock"

	"github.com/mini-maxit/worker/internal/pipeline"
	. "github.com/mini-maxit/worker/internal/scheduler"
	"github.com/mini-maxit/worker/pkg/constants"
	pkgerrors "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/messages"
	mocktests "github.com/mini-maxit/worker/tests/mocks"
)

func TestNewScheduler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	compiler := mocktests.NewMockCompiler(ctrl)
	packager := mocktests.NewMockPackager(ctrl)
	executor := mocktests.NewMockExecutor(ctrl)
	verifier := mocktests.NewMockVerifier(ctrl)
	responder := mocktests.NewMockResponder(ctrl)

	maxWorkers := 3
	s := NewScheduler(maxWorkers, compiler, packager, executor, verifier, responder)
	if s == nil {
		t.Fatalf("NewScheduler returned nil")
	}

	status := s.GetWorkersStatus()

	workers, ok := status["worker_status"].(map[int]string)
	if !ok {
		t.Fatalf("expected worker_status to be map[int]string, got %T", status["worker_status"])
	}
	if len(workers) != maxWorkers {
		t.Fatalf("expected %d workers, got %d", maxWorkers, len(workers))
	}
}

func TestGetWorkersStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	w0 := mocktests.NewMockWorker(ctrl)
	w1 := mocktests.NewMockWorker(ctrl)

	w0.EXPECT().GetStatus().Return(constants.WorkerStatusBusy).Times(1)
	w0.EXPECT().GetProcessingMessageID().Return("msg-1").Times(1)

	w1.EXPECT().GetStatus().Return(constants.WorkerStatusIdle).Times(1)

	s := NewSchedulerWithWorkers(2, map[int]pipeline.Worker{0: w0, 1: w1}, nil, nil, nil, nil, nil)

	st := s.GetWorkersStatus()
	if st == nil {
		t.Fatalf("expected non-nil status map")
	}

	totalWorkers, ok := st["total_workers"].(int)
	if !ok {
		t.Fatalf("expected total_workers to be int, got %T", st["total_workers"])
	}
	if totalWorkers != 2 {
		t.Fatalf("expected total_workers 2, got %v", totalWorkers)
	}
}

func TestProcessTask_SuccessAndMarkIdle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	w := mocktests.NewMockWorker(ctrl)

	// Scheduler will call GetStatus to find free worker
	w.EXPECT().GetStatus().Return(constants.WorkerStatusIdle).Times(1)
	// When a free worker is found, scheduler should call UpdateStatus(Busy)
	w.EXPECT().UpdateStatus(constants.WorkerStatusBusy).Times(1)

	w.EXPECT().GetId().Return(0).Times(2)

	// Expect ProcessTask to be invoked; signal when called
	done := make(chan struct{}, 1)
	w.EXPECT().ProcessTask(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Do(func(_ string, _ string, _ *messages.TaskQueueMessage) {
		// simulate some work
		time.Sleep(5 * time.Millisecond)
		select {
		case done <- struct{}{}:
		default:
		}
	}).Times(1)

	// After processing, scheduler.markWorkerAsIdle should call UpdateStatus(Idle)
	w.EXPECT().UpdateStatus(constants.WorkerStatusIdle).Times(1)

	s := NewSchedulerWithWorkers(1, map[int]pipeline.Worker{0: w}, nil, nil, nil, nil, nil)

	if err := s.ProcessTask("resp", "msg-id-1", &messages.TaskQueueMessage{}); err != nil {
		t.Fatalf("unexpected error from ProcessTask: %v", err)
	}

	// wait for ProcessTask to be called
	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for mocked worker ProcessTask to be called")
	}

	// give scheduler time to call markWorkerAsIdle
	time.Sleep(10 * time.Millisecond)
}

func TestProcessTask_NoFreeWorker(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	w := mocktests.NewMockWorker(ctrl)
	// worker reports busy
	w.EXPECT().GetStatus().Return(constants.WorkerStatusBusy).Times(1)

	s := NewSchedulerWithWorkers(1, map[int]pipeline.Worker{0: w}, nil, nil, nil, nil, nil)

	err := s.ProcessTask("resp", "msg-id-2", &messages.TaskQueueMessage{})
	if err == nil {
		t.Fatalf("expected error when no free worker available")
	}
	if !errors.Is(err, pkgerrors.ErrFailedToGetFreeWorker) {
		t.Fatalf("expected ErrFailedToGetFreeWorker, got %v", err)
	}
}
