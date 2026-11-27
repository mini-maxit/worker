package scheduler_test

import (
	"errors"
	"sync/atomic"
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

	packager := mocktests.NewMockPackager(ctrl)
	executor := mocktests.NewMockExecutor(ctrl)
	verifier := mocktests.NewMockVerifier(ctrl)
	responder := mocktests.NewMockResponder(ctrl)

	maxWorkers := 3
	s := NewScheduler(maxWorkers, packager, executor, verifier, responder)
	if s == nil {
		t.Fatalf("NewScheduler returned nil")
	}

	status := s.GetWorkersStatus()

	if len(status.WorkerStatus) != maxWorkers {
		t.Fatalf("expected %d workers, got %d", maxWorkers, len(status.WorkerStatus))
	}
}

func TestGetWorkersStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	w0 := mocktests.NewMockWorker(ctrl)
	w1 := mocktests.NewMockWorker(ctrl)

	w0.EXPECT().GetState().Return(
		pipeline.WorkerState{Status: constants.WorkerStatusBusy, ProcessingMessageID: "msg-1"},
	).Times(1)
	w0.EXPECT().GetId().Return(0).Times(1)

	w1.EXPECT().GetState().Return(pipeline.WorkerState{Status: constants.WorkerStatusIdle}).Times(1)
	w1.EXPECT().GetId().Return(1).Times(1)

	s := NewSchedulerWithWorkers(2, map[int]pipeline.Worker{0: w0, 1: w1}, nil, nil, nil, nil)

	st := s.GetWorkersStatus()
	if len(st.WorkerStatus) != 2 {
		t.Fatalf("expected 2 workers status, got %d", len(st.WorkerStatus))
	}

	busyFound := false
	idleFound := false
	for _, ws := range st.WorkerStatus {
		if ws.Status == constants.WorkerStatusBusy && ws.ProcessingMessageID == "msg-1" {
			busyFound = true
		}
		if ws.Status == constants.WorkerStatusIdle {
			idleFound = true
		}
	}

	if !busyFound {
		t.Fatalf("expected a busy worker processing message ID msg-1, none found")
	}

	if !idleFound {
		t.Fatalf("expected an idle worker, none found")
	}
}

func TestProcessTask_SuccessAndMarkIdle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	w := mocktests.NewMockWorker(ctrl)

	// Scheduler will call GetState to find free worker
	w.EXPECT().GetState().Return(pipeline.WorkerState{Status: constants.WorkerStatusIdle}).Times(1)
	// When a free worker is found, scheduler should call UpdateStatus(Busy)
	w.EXPECT().UpdateStatus(constants.WorkerStatusBusy).Times(1)

	w.EXPECT().GetId().Return(0).Times(1)

	publishErr := errors.New("publish failed")
	process := w.EXPECT().ProcessTask("msg-id-1", "resp", gomock.Any()).Return(publishErr).Times(1)

	// After processing, scheduler.markWorkerAsIdle should call UpdateStatus(Idle)
	var idle atomic.Bool
	w.EXPECT().UpdateStatus(constants.WorkerStatusIdle).Do(func(constants.WorkerStatus) {
		idle.Store(true)
	}).After(process).Times(1)

	s := NewSchedulerWithWorkers(1, map[int]pipeline.Worker{0: w}, nil, nil, nil, nil)

	result := make(chan error, 1)
	err := s.ProcessTask("resp", "msg-id-1", &messages.TaskQueueMessage{}, func(taskErr error) {
		// done acks the task, so the slot must already be free for the next delivery.
		if !idle.Load() {
			t.Error("done called before the worker was marked idle")
		}
		result <- taskErr
	})
	if err != nil {
		t.Fatalf("unexpected error from ProcessTask: %v", err)
	}

	select {
	case got := <-result:
		if !errors.Is(got, publishErr) {
			t.Fatalf("expected done to get the worker's error, got %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for done")
	}
}

func TestProcessTask_PanicReportsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	w := mocktests.NewMockWorker(ctrl)
	w.EXPECT().GetState().Return(pipeline.WorkerState{Status: constants.WorkerStatusIdle})
	w.EXPECT().UpdateStatus(constants.WorkerStatusBusy)
	w.EXPECT().ProcessTask(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(_ string, _ string, _ *messages.TaskQueueMessage) { panic("boom") })
	w.EXPECT().GetId().Return(0)
	w.EXPECT().UpdateStatus(constants.WorkerStatusIdle)

	s := NewSchedulerWithWorkers(1, map[int]pipeline.Worker{0: w}, nil, nil, nil, nil)

	result := make(chan error, 1)
	if err := s.ProcessTask("resp", "msg-id-3", &messages.TaskQueueMessage{}, func(e error) { result <- e }); err != nil {
		t.Fatalf("unexpected error from ProcessTask: %v", err)
	}

	select {
	case got := <-result:
		if !errors.Is(got, pkgerrors.ErrWorkerPanicked) {
			t.Fatalf("expected ErrWorkerPanicked, got %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for done")
	}
}

func TestProcessTask_NoFreeWorker(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	w := mocktests.NewMockWorker(ctrl)
	// worker reports busy
	w.EXPECT().GetState().Return(pipeline.WorkerState{Status: constants.WorkerStatusBusy}).Times(1)

	s := NewSchedulerWithWorkers(1, map[int]pipeline.Worker{0: w}, nil, nil, nil, nil)

	err := s.ProcessTask("resp", "msg-id-2", &messages.TaskQueueMessage{}, func(error) {
		t.Error("done must not be called for a rejected task")
	})
	if err == nil {
		t.Fatalf("expected error when no free worker available")
	}
	if !errors.Is(err, pkgerrors.ErrFailedToGetFreeWorker) {
		t.Fatalf("expected ErrFailedToGetFreeWorker, got %v", err)
	}
}
