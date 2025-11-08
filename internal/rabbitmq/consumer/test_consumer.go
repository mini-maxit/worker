//nolint:funlen
package consumer

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"go.uber.org/mock/gomock"

	mock_channel "github.com/mini-maxit/worker/tests/mocks/channel"
	mock_responder "github.com/mini-maxit/worker/tests/mocks/responder"
	mock_scheduler "github.com/mini-maxit/worker/tests/mocks/scheduler"

	"github.com/mini-maxit/worker/pkg/constants"
	pkgerrors "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
)

const workerQueue = "worker_queue_test"

func TestProcessMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockScheduler := mock_scheduler.NewMockScheduler(ctrl)
	mockResponder := mock_responder.NewMockResponder(ctrl)

	cIface := NewConsumer(nil, workerQueue, mockScheduler, mockResponder)
	c, ok := cIface.(*consumer)
	if !ok {
		t.Fatalf("NewConsumer returned unexpected type: %T", cIface)
	}

	t.Run("invalid json", func(t *testing.T) {
		// Expect responder.PublishErrorToResponseQueue called with empty type/messageID and the replyTo
		mockResponder.EXPECT().PublishErrorToResponseQueue("", "", "reply", gomock.Any()).Times(1)

		msg := amqp.Delivery{Body: []byte("not json"), ReplyTo: "reply"}
		c.processMessage(msg)
	})

	t.Run("unknown type", func(t *testing.T) {
		qm := messages.QueueMessage{Type: "foo", MessageID: "mid", Payload: nil}
		b, _ := json.Marshal(qm)

		mockResponder.EXPECT().PublishErrorToResponseQueue("foo", "mid", "reply", pkgerrors.ErrUnknownMessageType).Times(1)

		msg := amqp.Delivery{Body: b, ReplyTo: "reply"}
		c.processMessage(msg)
	})

	t.Run("task success", func(t *testing.T) {
		task := messages.TaskQueueMessage{LanguageType: "CPP", LanguageVersion: "17"}
		taskB, _ := json.Marshal(&task)
		qm := messages.QueueMessage{Type: constants.QueueMessageTypeTask, MessageID: "task-id-1", Payload: taskB}
		b, _ := json.Marshal(qm)

		mockScheduler.EXPECT().ProcessTask(
			"reply", "task-id-1", gomock.AssignableToTypeOf(&messages.TaskQueueMessage{}),
		).Return(nil).Times(1)

		msg := amqp.Delivery{Body: b, ReplyTo: "reply"}
		c.processMessage(msg)
	})

	t.Run("task requeue when no worker", func(t *testing.T) {
		task := messages.TaskQueueMessage{LanguageType: "CPP", LanguageVersion: "17"}
		taskB, _ := json.Marshal(&task)
		qm := messages.QueueMessage{Type: constants.QueueMessageTypeTask, MessageID: "task-id-2", Payload: taskB}
		b, _ := json.Marshal(qm)

		// Scheduler returns ErrFailedToGetFreeWorker
		mockScheduler.EXPECT().ProcessTask(
			"reply", "task-id-2", gomock.AssignableToTypeOf(&messages.TaskQueueMessage{}),
		).Return(pkgerrors.ErrFailedToGetFreeWorker).Times(1)

		// Expect responder.Publish called to requeue with higher priority. Inspect publishing in Do.
		mockResponder.EXPECT().Publish(
			workerQueue, gomock.AssignableToTypeOf(amqp.Publishing{}),
		).Do(func(_ string, p amqp.Publishing) {
			if p.Priority != uint8(constants.RabbitMQRequeuePriority) {
				t.Fatalf("expected Priority to be %d got %d", constants.RabbitMQRequeuePriority, p.Priority)
			}
		}).Return(nil).Times(1)

		msg := amqp.Delivery{Body: b, ReplyTo: "reply"}
		c.processMessage(msg)
	})

	t.Run("status success", func(t *testing.T) {
		status := map[string]any{"w1": "idle"}
		qm := messages.QueueMessage{Type: constants.QueueMessageTypeStatus, MessageID: "status-id", Payload: nil}
		b, _ := json.Marshal(qm)

		mockScheduler.EXPECT().GetWorkersStatus().Return(status).Times(1)
		mockResponder.EXPECT().PublishSucessStatusRespond(
			constants.QueueMessageTypeStatus, "status-id", "reply", status,
		).Return(nil).Times(1)

		msg := amqp.Delivery{Body: b, ReplyTo: "reply"}
		c.processMessage(msg)
	})

	t.Run("handshake success", func(t *testing.T) {
		qm := messages.QueueMessage{Type: constants.QueueMessageTypeHandshake, MessageID: "hs-id", Payload: nil}
		b, _ := json.Marshal(qm)

		// Expect responder to be called with a slice of LanguageSpec. We can't directly Eq the slice, so inspect in Do.
		mockResponder.EXPECT().PublishSucessHandshakeRespond(
			constants.QueueMessageTypeHandshake, "hs-id", "reply", gomock.AssignableToTypeOf([]languages.LanguageSpec{}),
		).Do(func(_ string, _ string, _ string, langs []languages.LanguageSpec) {
			if len(langs) == 0 {
				t.Fatalf("expected at least one language in handshake payload")
			}
		}).Return(nil).Times(1)

		msg := amqp.Delivery{Body: b, ReplyTo: "reply"}
		c.processMessage(msg)
	})

	t.Run("task success", func(t *testing.T) {
		task := messages.TaskQueueMessage{LanguageType: "CPP", LanguageVersion: "17"}
		taskB, _ := json.Marshal(&task)
		qm := messages.QueueMessage{Type: constants.QueueMessageTypeTask, MessageID: "task-id-1", Payload: taskB}
		b, _ := json.Marshal(qm)

		mockScheduler.EXPECT().ProcessTask(
			"reply", "task-id-1", gomock.AssignableToTypeOf(&messages.TaskQueueMessage{}),
		).Return(nil).Times(1)

		msg := amqp.Delivery{Body: b, ReplyTo: "reply"}
		c.processMessage(msg)
	})
}

func TestListen_ProcessTaskMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChannel := mock_channel.NewMockChannel(ctrl)
	mockScheduler := mock_scheduler.NewMockScheduler(ctrl)
	mockResponder := mock_responder.NewMockResponder(ctrl)

	// deliveries channel that will be returned by Consume
	deliveries := make(chan amqp.Delivery)

	// Expect QueueDeclare and verify x-max-priority is set
	mockChannel.EXPECT().QueueDeclare(workerQueue, true, false, false, false, gomock.AssignableToTypeOf(amqp.Table{})).Do(
		func(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) {
			v, ok := args["x-max-priority"]
			if !ok {
				t.Fatalf("expected x-max-priority to be present in args")
			}
			if v != constants.RabbitMQMaxPriority {
				t.Fatalf("expected x-max-priority %v got %v", constants.RabbitMQMaxPriority, v)
			}
		}).Return(amqp.Queue{Name: workerQueue}, nil).Times(1)

	// Expect Consume to be called and return our deliveries channel
	mockChannel.EXPECT().Consume(
		workerQueue, "", true, false, false, false, nil,
	).Return((<-chan amqp.Delivery)(deliveries), nil).Times(1)

	// When the scheduler's ProcessTask is called, signal completion
	done := make(chan struct{})
	mockScheduler.EXPECT().ProcessTask(
		"reply", "task-id-listen", gomock.AssignableToTypeOf(&messages.TaskQueueMessage{}),
	).Do(
		func(_ string, _ string, _ *messages.TaskQueueMessage) {
			select {
			case done <- struct{}{}:
			default:
			}
		}).Return(nil).Times(1)

	cIface := NewConsumer(mockChannel, workerQueue, mockScheduler, mockResponder)
	c, ok := cIface.(*consumer)
	if !ok {
		t.Fatalf("NewConsumer returned unexpected type: %T", cIface)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Listen will range over deliveries until it's closed
		c.Listen()
	}()

	// Build a task message and send it
	task := messages.TaskQueueMessage{LanguageType: "CPP", LanguageVersion: "17"}
	taskB, _ := json.Marshal(&task)
	qm := messages.QueueMessage{Type: constants.QueueMessageTypeTask, MessageID: "task-id-listen", Payload: taskB}
	b, _ := json.Marshal(qm)

	deliveries <- amqp.Delivery{Body: b, ReplyTo: "reply"}

	// Wait for process to be invoked
	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for ProcessTask to be called")
	}

	// Close deliveries so Listen can return and goroutine exits
	close(deliveries)
	// Wait for Listen goroutine to finish
	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for Listen to finish")
	}
}

func TestListen_QueueDeclareErrorPanics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChannel := mock_channel.NewMockChannel(ctrl)
	mockScheduler := mock_scheduler.NewMockScheduler(ctrl)
	mockResponder := mock_responder.NewMockResponder(ctrl)

	mockChannel.EXPECT().QueueDeclare(
		workerQueue, true, false, false, false, gomock.Any(),
	).Return(amqp.Queue{}, errors.New("queue error")).Times(1)

	cIface := NewConsumer(mockChannel, workerQueue, mockScheduler, mockResponder)
	c, ok := cIface.(*consumer)
	if !ok {
		t.Fatalf("NewConsumer returned unexpected type: %T", cIface)
	}

	// Expect a panic from Panicf call inside Listen
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected Listen to panic on QueueDeclare error")
		}
	}()

	c.Listen()
}

func TestListen_ConsumeErrorPanics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChannel := mock_channel.NewMockChannel(ctrl)
	mockScheduler := mock_scheduler.NewMockScheduler(ctrl)
	mockResponder := mock_responder.NewMockResponder(ctrl)

	mockChannel.EXPECT().QueueDeclare(
		workerQueue, true, false, false, false, gomock.Any(),
	).Return(amqp.Queue{Name: workerQueue}, nil).Times(1)
	mockChannel.EXPECT().Consume(
		workerQueue, "", true, false, false, false, nil,
	).Return((<-chan amqp.Delivery)(nil), errors.New("consume error")).Times(1)

	cIface := NewConsumer(mockChannel, workerQueue, mockScheduler, mockResponder)
	c, ok := cIface.(*consumer)
	if !ok {
		t.Fatalf("NewConsumer returned unexpected type: %T", cIface)
	}

	// Expect a panic from Panicf call inside Listen
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected Listen to panic on Consume error")
		}
	}()

	c.Listen()
}
