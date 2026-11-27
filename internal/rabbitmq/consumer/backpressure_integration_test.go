//go:build integration

package consumer_test

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/internal/pipeline"
	"github.com/mini-maxit/worker/internal/rabbitmq"
	"github.com/mini-maxit/worker/internal/rabbitmq/consumer"
	"github.com/mini-maxit/worker/internal/rabbitmq/responder"
	"github.com/mini-maxit/worker/internal/scheduler"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
	amqp "github.com/rabbitmq/amqp091-go"
)

// blockingWorker holds its slot until the test sends on release, then publishes a result.
type blockingWorker struct {
	pipeline.Worker
	started   chan string
	release   chan struct{}
	responder responder.Responder
}

func (w *blockingWorker) ProcessTask(messageID, responseQueue string, _ *messages.TaskQueueMessage) error {
	w.started <- messageID
	<-w.release
	return w.responder.PublishPayloadTaskRespond(
		constants.QueueMessageTypeTask, messageID, responseQueue, solution.Result{})
}

type countingScheduler struct {
	scheduler.Scheduler
	calls atomic.Int64
}

func (s *countingScheduler) ProcessTask(
	responseQueue, messageID string, task *messages.TaskQueueMessage, done func(error),
) error {
	s.calls.Add(1)
	return s.Scheduler.ProcessTask(responseQueue, messageID, task, done)
}

type testWorkerService struct {
	conn      *amqp.Connection
	scheduler *countingScheduler
}

func startConsumer(
	t *testing.T, queue string, slots int, started chan string, release chan struct{},
) testWorkerService {
	t.Helper()
	conn := rabbitmq.NewRabbitMqConnection(&config.Config{RabbitMQURL: testRabbitMQURL})
	t.Cleanup(func() { _ = conn.Close() })
	ch := rabbitmq.NewRabbitMQChannel(conn)
	resp := responder.NewResponder(ch, constants.DefaultRabbitmqPublishChanSize)

	workers := make(map[int]pipeline.Worker, slots)
	for i := range slots {
		workers[i] = &blockingWorker{
			Worker:    pipeline.NewWorker(i, nil, nil, nil, resp),
			started:   started,
			release:   release,
			responder: resp,
		}
	}
	s := &countingScheduler{Scheduler: scheduler.NewSchedulerWithWorkers(slots, workers, nil, nil, nil, resp)}
	go consumer.NewConsumer(ch, queue, slots, s, resp).Listen()
	return testWorkerService{conn: conn, scheduler: s}
}

// testBackend publishes to the worker queue and reads the reply queue, as the backend does.
type testBackend struct {
	t       *testing.T
	ch      *amqp.Channel
	queue   string
	reply   string
	replies <-chan amqp.Delivery
}

func newTestBackend(t *testing.T) testBackend {
	t.Helper()
	conn, err := amqp.Dial(testRabbitMQURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	queue := fmt.Sprintf("test_backpressure_%d", time.Now().UnixNano())
	// Declared here too, so that publishing cannot race the consumer's declaration.
	args := amqp.Table{"x-max-priority": constants.RabbitMQMaxPriority}
	if _, err := ch.QueueDeclare(queue, true, false, false, false, args); err != nil {
		t.Fatalf("declare queue: %v", err)
	}
	reply, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatalf("declare reply queue: %v", err)
	}
	replies, err := ch.Consume(reply.Name, "", true, true, false, false, nil)
	if err != nil {
		t.Fatalf("consume replies: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ch.QueueDelete(queue, false, false, false)
		_, _ = ch.QueueDelete(queue+constants.TaskQueueSuffix, false, false, false)
		_ = conn.Close()
	})
	return testBackend{t: t, ch: ch, queue: queue, reply: reply.Name, replies: replies}
}

func (b testBackend) publish(messageType, messageID string) {
	b.t.Helper()
	body, err := json.Marshal(messages.QueueMessage{Type: messageType, MessageID: messageID, Payload: []byte("{}")})
	if err != nil {
		b.t.Fatalf("marshal: %v", err)
	}
	if err := b.ch.Publish("", b.queue, false, false, amqp.Publishing{Body: body, ReplyTo: b.reply}); err != nil {
		b.t.Fatalf("publish: %v", err)
	}
}

func (b testBackend) nextReply() messages.ResponseQueueMessage {
	b.t.Helper()
	var response messages.ResponseQueueMessage
	select {
	case m := <-b.replies:
		if err := json.Unmarshal(m.Body, &response); err != nil {
			b.t.Fatalf("unmarshal reply: %v", err)
		}
	case <-time.After(5 * time.Second):
		b.t.Fatal("timed out waiting for a reply")
	}
	return response
}

func (b testBackend) readyMessages(queue string) int {
	b.t.Helper()
	// The arguments only have to name an existing queue: a passive declare changes nothing.
	q, err := b.ch.QueueDeclarePassive(queue, true, false, false, false, nil)
	if err != nil {
		b.t.Fatalf("inspect %s: %v", queue, err)
	}
	return q.Messages
}

func waitStarted(t *testing.T, started chan string) string {
	t.Helper()
	select {
	case id := <-started:
		return id
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a task to start")
		return ""
	}
}

func TestConsumer_AllSlotsBusy(t *testing.T) {
	const slots, tasks = 2, 6
	backend := newTestBackend(t)
	started := make(chan string, tasks)
	release := make(chan struct{})
	worker := startConsumer(t, backend.queue, slots, started, release)

	for i := range tasks {
		backend.publish(constants.QueueMessageTypeTask, fmt.Sprintf("task-%d", i))
	}
	for range slots {
		waitStarted(t, started)
	}

	// Control messages sent behind the backlog are answered while every slot is busy.
	for _, messageType := range []string{constants.QueueMessageTypeStatus, constants.QueueMessageTypeHandshake} {
		backend.publish(messageType, messageType)
		if got := backend.nextReply(); got.Type != messageType || !got.Ok {
			t.Fatalf("expected a %s reply, got %+v", messageType, got)
		}
	}

	// Waiting tasks stay in the broker instead of cycling through the worker.
	time.Sleep(500 * time.Millisecond)
	if got := worker.scheduler.calls.Load(); got != slots {
		t.Fatalf("expected %d scheduler calls while all slots are busy, got %d", slots, got)
	}
	if waiting := backend.readyMessages(backend.queue + constants.TaskQueueSuffix); waiting != tasks-slots {
		t.Fatalf("expected %d tasks waiting in the broker, got %d", tasks-slots, waiting)
	}

	// Every task, including those that had to wait, is answered on its reply queue exactly once.
	close(release)
	answered := map[string]bool{}
	for range tasks {
		got := backend.nextReply()
		if got.Type != constants.QueueMessageTypeTask || answered[got.MessageID] {
			t.Fatalf("unexpected reply %+v", got)
		}
		answered[got.MessageID] = true
	}
	if got := worker.scheduler.calls.Load(); got != tasks {
		t.Fatalf("expected %d scheduler calls in total, got %d", tasks, got)
	}
}

func TestConsumer_TaskRedeliveredWhenWorkerDies(t *testing.T) {
	backend := newTestBackend(t)
	started := make(chan string, 4)
	dead := make(chan struct{}) // the first worker never finishes
	first := startConsumer(t, backend.queue, 1, started, dead)

	backend.publish(constants.QueueMessageTypeTask, "task-running")
	backend.publish(constants.QueueMessageTypeTask, "task-waiting")
	waitStarted(t, started)

	if err := first.conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	release := make(chan struct{})
	close(release)
	second := startConsumer(t, backend.queue, 1, started, release)

	answered := map[string]bool{}
	for range 2 {
		answered[backend.nextReply().MessageID] = true
	}
	if !answered["task-running"] || !answered["task-waiting"] {
		t.Fatalf("expected both tasks to be answered, got %v", answered)
	}

	// Both are acknowledged: nothing returns to the queues when the consumer goes away.
	time.Sleep(200 * time.Millisecond)
	if err := second.conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	for _, name := range []string{backend.queue, backend.queue + constants.TaskQueueSuffix} {
		if left := backend.readyMessages(name); left != 0 {
			t.Fatalf("expected %s to be empty, got %d messages", name, left)
		}
	}
}
