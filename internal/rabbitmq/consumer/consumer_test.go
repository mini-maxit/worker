package consumer_test

import (
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"go.uber.org/mock/gomock"

	"github.com/mini-maxit/worker/tests/mocks"

	"github.com/mini-maxit/worker/internal/rabbitmq/consumer"
	"github.com/mini-maxit/worker/pkg/constants"
	pkgerrors "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/messages"
)

const (
	workerQueue = "worker_queue_test"
	testReplyTo = "reply"
	testLang    = "CPP"
	taskQueue   = workerQueue + constants.TaskQueueSuffix
)

// fakeAck records how a delivery was settled.
type fakeAck struct{ calls []string }

func (f *fakeAck) Ack(_ uint64, _ bool) error { f.calls = append(f.calls, "ack"); return nil }

func (f *fakeAck) Nack(_ uint64, _ bool, requeue bool) error {
	if requeue {
		f.calls = append(f.calls, "requeue")
	} else {
		f.calls = append(f.calls, "drop")
	}
	return nil
}

func (f *fakeAck) Reject(tag uint64, requeue bool) error { return f.Nack(tag, false, requeue) }

func (f *fakeAck) expect(t *testing.T, want ...string) {
	t.Helper()
	if !slices.Equal(f.calls, want) {
		t.Fatalf("expected delivery settlement %v, got %v", want, f.calls)
	}
}

func taskBody(t *testing.T, messageID string) []byte {
	t.Helper()
	payload, err := json.Marshal(messages.TaskQueueMessage{LanguageType: testLang, LanguageVersion: "17"})
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	body, err := json.Marshal(messages.QueueMessage{
		Type: constants.QueueMessageTypeTask, MessageID: messageID, Payload: payload,
	})
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	return body
}

func TestProcessMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockScheduler := mocks.NewMockScheduler(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	c := consumer.NewConsumer(nil, workerQueue, 1, mockScheduler, mockResponder)

	t.Run("invalid json", func(t *testing.T) {
		// Expect responder.PublishErrorToResponseQueue called with empty type/messageID and the replyTo
		mockResponder.EXPECT().PublishErrorToResponseQueue("", "", testReplyTo, gomock.Any()).Times(1)

		msg := amqp.Delivery{Body: []byte("not json"), ReplyTo: testReplyTo}
		c.ProcessMessage(msg)
	})

	t.Run("unknown type", func(t *testing.T) {
		qm := messages.QueueMessage{Type: "foo", MessageID: "mid", Payload: nil}
		b, _ := json.Marshal(qm)

		mockResponder.
			EXPECT().
			PublishErrorToResponseQueue("foo", "mid", testReplyTo, pkgerrors.ErrUnknownMessageType).
			Times(1)

		msg := amqp.Delivery{Body: b, ReplyTo: testReplyTo}
		c.ProcessMessage(msg)
	})

	t.Run("task is moved to the task queue", func(t *testing.T) {
		body := taskBody(t, "task-id-1")
		ack := &fakeAck{}

		mockResponder.EXPECT().Publish(taskQueue, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			ReplyTo:      testReplyTo,
			Priority:     2,
			Body:         body,
		}).Return(nil).Times(1)

		c.ProcessMessage(amqp.Delivery{Acknowledger: ack, Body: body, ReplyTo: testReplyTo, Priority: 2})
		ack.expect(t, "ack")
	})

	t.Run("task stays queued when it cannot be moved", func(t *testing.T) {
		ack := &fakeAck{}
		mockResponder.EXPECT().Publish(taskQueue, gomock.Any()).Return(errors.New("closed")).Times(1)

		c.ProcessMessage(amqp.Delivery{Acknowledger: ack, Body: taskBody(t, "task-id-2")})
		ack.expect(t, "requeue")
	})

	t.Run("status success", func(t *testing.T) {
		status := messages.ResponseWorkerStatusPayload{
			BusyWorkers:  1,
			TotalWorkers: 1,
			WorkerStatus: []messages.WorkerStatus{{WorkerID: 1, Status: constants.WorkerStatusIdle}},
		}
		qm := messages.QueueMessage{Type: constants.QueueMessageTypeStatus, MessageID: "status-id", Payload: nil}
		b, _ := json.Marshal(qm)

		mockScheduler.EXPECT().GetWorkersStatus().Return(status).Times(1)
		mockResponder.EXPECT().PublishSuccessStatusRespond(
			constants.QueueMessageTypeStatus, "status-id", testReplyTo, status,
		).Times(1)

		msg := amqp.Delivery{Body: b, ReplyTo: testReplyTo}
		c.ProcessMessage(msg)
	})

	t.Run("handshake success", func(t *testing.T) {
		qm := messages.QueueMessage{Type: constants.QueueMessageTypeHandshake, MessageID: "hs-id", Payload: nil}
		b, _ := json.Marshal(qm)

		// Expect responder to be called with ResponseHandshakePayload. Inspect in Do.
		mockResponder.EXPECT().PublishSuccessHandshakeRespond(
			constants.QueueMessageTypeHandshake,
			"hs-id",
			testReplyTo,
			gomock.AssignableToTypeOf(messages.ResponseHandshakePayload{}),
		).Do(func(_ string, _ string, _ string, langs messages.ResponseHandshakePayload) {
			if len(langs.Languages) == 0 {
				t.Fatalf("expected at least one language in handshake payload")
			}
		}).Times(1)

		msg := amqp.Delivery{Body: b, ReplyTo: testReplyTo}
		c.ProcessMessage(msg)
	})
}

func TestProcessTaskDelivery(t *testing.T) {
	newConsumer := func(t *testing.T) (consumer.Consumer, *mocks.MockScheduler, *mocks.MockResponder) {
		ctrl := gomock.NewController(t)
		s := mocks.NewMockScheduler(ctrl)
		r := mocks.NewMockResponder(ctrl)
		return consumer.NewConsumer(nil, workerQueue, 1, s, r), s, r
	}

	for _, tc := range []struct {
		name    string
		taskErr error
		want    string
	}{
		{name: "acked after the response is published", taskErr: nil, want: "ack"},
		{name: "requeued when no response is published", taskErr: errors.New("closed"), want: "requeue"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, s, _ := newConsumer(t)
			ack := &fakeAck{}
			var done func(error)
			s.EXPECT().ProcessTask(testReplyTo, "task-id", gomock.Any(), gomock.Any()).DoAndReturn(
				func(_, _ string, _ *messages.TaskQueueMessage, d func(error)) error {
					done = d
					return nil
				})

			c.ProcessTaskDelivery(amqp.Delivery{Acknowledger: ack, Body: taskBody(t, "task-id"), ReplyTo: testReplyTo})
			ack.expect(t) // still running

			done(tc.taskErr)
			ack.expect(t, tc.want)
		})
	}

	t.Run("answered and acked when the worker panicked", func(t *testing.T) {
		c, s, r := newConsumer(t)
		ack := &fakeAck{}
		var done func(error)
		s.EXPECT().ProcessTask(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_, _ string, _ *messages.TaskQueueMessage, d func(error)) error {
				done = d
				return nil
			})
		r.EXPECT().PublishErrorToResponseQueue(constants.QueueMessageTypeTask, "task-id", testReplyTo, gomock.Any())

		c.ProcessTaskDelivery(amqp.Delivery{Acknowledger: ack, Body: taskBody(t, "task-id"), ReplyTo: testReplyTo})
		done(pkgerrors.ErrWorkerPanicked)
		ack.expect(t, "ack")
	})

	t.Run("requeued when no worker is free", func(t *testing.T) {
		c, s, _ := newConsumer(t)
		ack := &fakeAck{}
		s.EXPECT().ProcessTask(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(pkgerrors.ErrFailedToGetFreeWorker)

		c.ProcessTaskDelivery(amqp.Delivery{Acknowledger: ack, Body: taskBody(t, "task-id")})
		ack.expect(t, "requeue")
	})

	t.Run("invalid payload is answered and acked", func(t *testing.T) {
		c, _, r := newConsumer(t)
		ack := &fakeAck{}
		body, _ := json.Marshal(messages.QueueMessage{
			Type: constants.QueueMessageTypeTask, MessageID: "bad", Payload: json.RawMessage(`"not a task"`),
		})
		r.EXPECT().PublishErrorToResponseQueue(constants.QueueMessageTypeTask, "bad", testReplyTo, gomock.Any())

		c.ProcessTaskDelivery(amqp.Delivery{Acknowledger: ack, Body: body, ReplyTo: testReplyTo})
		ack.expect(t, "ack")
	})

	t.Run("dropped on panic", func(t *testing.T) {
		c, s, _ := newConsumer(t)
		ack := &fakeAck{}
		s.EXPECT().ProcessTask(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(_, _ string, _ *messages.TaskQueueMessage, _ func(error)) { panic("boom") })

		c.ProcessTaskDelivery(amqp.Delivery{Acknowledger: ack, Body: taskBody(t, "task-id")})
		ack.expect(t, "drop")
	})
}

func TestListen_ProcessTaskMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChannel := mocks.NewMockChannel(ctrl)
	mockScheduler := mocks.NewMockScheduler(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	// delivery channels that will be returned by Consume
	deliveries := make(chan amqp.Delivery)
	taskDeliveries := make(chan amqp.Delivery)

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

	mockChannel.EXPECT().QueueDeclare(taskQueue, true, false, false, false, gomock.AssignableToTypeOf(amqp.Table{})).
		Return(amqp.Queue{Name: taskQueue}, nil).Times(1)

	// The prefetch limit must be set between the two consumers so that it applies to the task queue only.
	started := make(chan struct{})
	gomock.InOrder(
		mockChannel.EXPECT().Consume(workerQueue, "", false, false, false, false, nil).
			Return((<-chan amqp.Delivery)(deliveries), nil),
		mockChannel.EXPECT().Qos(3, 0, false).Return(nil),
		mockChannel.EXPECT().Consume(taskQueue, "", false, false, false, false, nil).
			Do(func(_ string, _ string, _ bool, _ bool, _ bool, _ bool, _ amqp.Table) {
				// notify that Listen is ready to receive deliveries
				close(started)
			}).Return((<-chan amqp.Delivery)(taskDeliveries), nil),
	)

	// When the scheduler's ProcessTask is called, signal completion
	done := make(chan struct{}, 1)
	mockScheduler.EXPECT().ProcessTask(
		testReplyTo, "task-id-listen", gomock.AssignableToTypeOf(&messages.TaskQueueMessage{}), gomock.Any(),
	).Do(
		func(_ string, _ string, _ *messages.TaskQueueMessage, _ func(error)) {
			done <- struct{}{}
		}).Return(nil).Times(1)

	c := consumer.NewConsumer(mockChannel, workerQueue, 3, mockScheduler, mockResponder)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Listen will range over deliveries until it's closed
		c.Listen()
	}()

	// Wait for Listen to call Consume so the goroutine is ready to receive deliveries
	select {
	case <-started:
		// ready
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for Listen to start consuming")
	}

	taskDeliveries <- amqp.Delivery{Acknowledger: &fakeAck{}, Body: taskBody(t, "task-id-listen"), ReplyTo: testReplyTo}

	// Wait for process to be invoked
	select {
	case <-done:
		// good
	case <-time.After(30 * time.Second):
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
	case <-time.After(30 * time.Second):
		t.Fatalf("timed out waiting for Listen to finish")
	}
}

func TestListen_QueueDeclareErrorPanics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChannel := mocks.NewMockChannel(ctrl)
	mockScheduler := mocks.NewMockScheduler(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	mockChannel.EXPECT().QueueDeclare(
		workerQueue, true, false, false, false, gomock.Any(),
	).Return(amqp.Queue{}, errors.New("queue error")).Times(1)

	c := consumer.NewConsumer(mockChannel, workerQueue, 1, mockScheduler, mockResponder)

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

	mockChannel := mocks.NewMockChannel(ctrl)
	mockScheduler := mocks.NewMockScheduler(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	mockChannel.EXPECT().QueueDeclare(
		workerQueue, true, false, false, false, gomock.Any(),
	).Return(amqp.Queue{Name: workerQueue}, nil).Times(1)
	mockChannel.EXPECT().QueueDeclare(taskQueue, true, false, false, false, gomock.AssignableToTypeOf(amqp.Table{})).
		Return(amqp.Queue{Name: taskQueue}, nil).Times(1)
	mockChannel.EXPECT().Consume(
		workerQueue, "", false, false, false, false, nil,
	).Return((<-chan amqp.Delivery)(nil), errors.New("consume error")).Times(1)

	c := consumer.NewConsumer(mockChannel, workerQueue, 1, mockScheduler, mockResponder)
	// Expect a panic from Panicf call inside Listen
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected Listen to panic on Consume error")
		}
	}()

	c.Listen()
}
