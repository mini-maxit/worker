//nolint:gocognit
package responder

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/mock/gomock"

	mock_channel "github.com/mini-maxit/worker/tests/mocks/channel"

	pkgerrors "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
)

// Test PublishErrorToResponseQueue creates an error response and ensures it's published.
func TestPublishErrorToResponseQueue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCh := mock_channel.NewMockChannel(ctrl)
	r := NewResponder(mockCh, 10)
	defer func() {
		if err := r.Close(); err != nil {
			t.Fatalf("failed to close responder: %v", err)
		}
	}()

	messageType := "err-type"
	messageID := "mid-1"
	responseQueue := "resp-queue"
	testErr := errors.New("some error")

	mockCh.EXPECT().Publish("", responseQueue, false, false, gomock.AssignableToTypeOf(amqp.Publishing{})).Do(
		func(_ string, _ string, _ bool, _ bool, pub amqp.Publishing) {
			var resp messages.ResponseQueueMessage
			if err := json.Unmarshal(pub.Body, &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			if resp.Type != messageType {
				t.Fatalf("expected type %s got %s", messageType, resp.Type)
			}
			if resp.MessageID != messageID {
				t.Fatalf("expected messageID %s got %s", messageID, resp.MessageID)
			}
			if resp.Ok {
				t.Fatalf("expected Ok=false for error response")
			}
			var payload map[string]string
			if err := json.Unmarshal(resp.Payload, &payload); err != nil {
				t.Fatalf("failed to unmarshal payload: %v", err)
			}
			if payload["error"] != testErr.Error() {
				t.Fatalf("expected payload error %s got %s", testErr.Error(), payload["error"])
			}
			if pub.CorrelationId != messageID {
				t.Fatalf("expected CorrelationId %s got %s", messageID, pub.CorrelationId)
			}
		}).Return(nil).Times(1)

	r.PublishErrorToResponseQueue(messageType, messageID, responseQueue, testErr)
}

// Test publish helpers: status, handshake and payload task.
func TestPublishRespondHelpers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCh := mock_channel.NewMockChannel(ctrl)
	r := NewResponder(mockCh, 10)
	defer func() {
		if err := r.Close(); err != nil {
			t.Fatalf("failed to close responder: %v", err)
		}
	}()

	// Status
	statusMap := map[string]interface{}{"w1": "idle"}
	mockCh.EXPECT().Publish("", "status-queue", false, false, gomock.AssignableToTypeOf(amqp.Publishing{})).Do(
		func(_ string, _ string, _ bool, _ bool, pub amqp.Publishing) {
			var resp messages.ResponseQueueMessage
			if err := json.Unmarshal(pub.Body, &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			if !resp.Ok {
				t.Fatalf("expected Ok=true for status response")
			}
			// payload should match statusMap
			var got map[string]interface{}
			if err := json.Unmarshal(resp.Payload, &got); err != nil {
				t.Fatalf("failed to unmarshal payload: %v", err)
			}
			if got["w1"] != "idle" {
				t.Fatalf("expected status payload to contain w1=idle")
			}
		}).Return(nil).Times(1)

	if err := r.PublishSuccessStatusRespond("status", "sid", "status-queue", statusMap); err != nil {
		t.Fatalf("PublishSucessStatusRespond returned error: %v", err)
	}

	// Handshake
	langs := languages.GetSupportedLanguagesWithVersions()
	mockCh.EXPECT().Publish("", "hs-queue", false, false, gomock.AssignableToTypeOf(amqp.Publishing{})).Do(
		func(_ string, _ string, _ bool, _ bool, pub amqp.Publishing) {
			var resp messages.ResponseQueueMessage
			if err := json.Unmarshal(pub.Body, &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			if !resp.Ok {
				t.Fatalf("expected Ok=true for handshake response")
			}
			// handshakePayload wrapper
			var wrapper struct {
				Languages []languages.LanguageSpec `json:"languages"`
			}
			if err := json.Unmarshal(resp.Payload, &wrapper); err != nil {
				t.Fatalf("failed to unmarshal handshake payload: %v", err)
			}
			if len(wrapper.Languages) == 0 {
				t.Fatalf("expected at least one language in handshake payload")
			}
		}).Return(nil).Times(1)

	if err := r.PublishSuccessHandshakeRespond("hs", "hid", "hs-queue", langs); err != nil {
		t.Fatalf("PublishSucessHandshakeRespond returned error: %v", err)
	}

	// Payload task
	res := solution.Result{StatusCode: solution.Success, Message: "ok"}
	mockCh.EXPECT().Publish("", "task-queue", false, false, gomock.AssignableToTypeOf(amqp.Publishing{})).Do(
		func(_ string, _ string, _ bool, _ bool, pub amqp.Publishing) {
			var resp messages.ResponseQueueMessage
			if err := json.Unmarshal(pub.Body, &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			if !resp.Ok {
				t.Fatalf("expected Ok=true for task payload response")
			}
			var got solution.Result
			if err := json.Unmarshal(resp.Payload, &got); err != nil {
				t.Fatalf("failed to unmarshal task payload: %v", err)
			}
			if got.StatusCode != solution.Success {
				t.Fatalf("expected status code %v got %v", solution.Success, got.StatusCode)
			}
		}).Return(nil).Times(1)

	if err := r.PublishPayloadTaskRespond("task", "tid", "task-queue", res); err != nil {
		t.Fatalf("PublishPayloadTaskRespond returned error: %v", err)
	}
}

// Test concurrent heavy load of Publish calls.
func TestPublish_ConcurrentHighLoad(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCh := mock_channel.NewMockChannel(ctrl)
	// use bigger publish channel buffer to reduce blocking.
	r := NewResponder(mockCh, 1000)
	defer func() {
		if err := r.Close(); err != nil {
			t.Fatalf("failed to close responder: %v", err)
		}
	}()

	const n = 200

	// Expect n calls; collect bodies concurrently
	var mu sync.Mutex
	received := make(map[string]struct{})
	mockCh.EXPECT().Publish("", "q-heavy", false, false, gomock.AssignableToTypeOf(amqp.Publishing{})).Do(
		func(_ string, _ string, _ bool, _ bool, pub amqp.Publishing) {
			mu.Lock()
			received[string(pub.Body)] = struct{}{}
			mu.Unlock()
			// slight delay to exercise queuing
			time.Sleep(1 * time.Millisecond)
		}).Return(nil).Times(n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			body := []byte(fmt.Sprintf("msg-%d", i))
			if err := r.Publish("q-heavy", amqp.Publishing{ContentType: "text/plain", Body: body}); err != nil {
				t.Errorf("Publish returned error: %v", err)
			}
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for concurrent publishes to finish")
	}

	if len(received) != n {
		t.Fatalf("expected %d published messages, got %d", n, len(received))
	}
}

// Test that Publish returns the underlying channel publish error.
func TestPublish_ReturnsChannelError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCh := mock_channel.NewMockChannel(ctrl)
	r := NewResponder(mockCh, 10)
	defer func() {
		if err := r.Close(); err != nil {
			t.Fatalf("failed to close responder: %v", err)
		}
	}()

	expectedErr := errors.New("publish failed")
	mockCh.EXPECT().Publish(
		"", "err-q", false, false, gomock.AssignableToTypeOf(amqp.Publishing{}),
	).Return(expectedErr).Times(1)

	err := r.Publish("err-q", amqp.Publishing{Body: []byte("x")})
	if err == nil || err.Error() != expectedErr.Error() {
		t.Fatalf("expected error %v got %v", expectedErr, err)
	}
}

// Test that Close prevents Publish and returns ErrResponderClosed.
func TestClose_PreventsPublish(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCh := mock_channel.NewMockChannel(ctrl)
	r := NewResponder(mockCh, 10)

	if err := r.Close(); err != nil {
		t.Fatalf("unexpected error on close: %v", err)
	}

	// Now Publish should return ErrResponderClosed immediately
	err := r.Publish("any", amqp.Publishing{Body: []byte("x")})
	if err == nil {
		t.Fatalf("expected ErrResponderClosed got nil")
	}
	if !errors.Is(err, pkgerrors.ErrResponderClosed) {
		t.Fatalf("expected ErrResponderClosed got %v", err)
	}
}
