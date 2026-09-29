package eventbus

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	eb := New()
	if eb == nil {
		t.Fatal("New() returned nil")
	}
	if eb.handlers == nil {
		t.Error("handlers map not initialized")
	}
	if eb.buffer != 100 {
		t.Errorf("expected buffer 100, got %d", eb.buffer)
	}
}

func TestSubscribe(t *testing.T) {
	eb := New()
	called := false
	handler := func(ctx context.Context, event Event) error {
		called = true
		return nil
	}

	eb.Subscribe("test", handler)
	eb.Publish(context.Background(), Event{Type: "test"})

	if !called {
		t.Error("handler was not called")
	}
}

func TestSubscribeAll(t *testing.T) {
	eb := New()
	count := 0
	handler := func(ctx context.Context, event Event) error {
		count++
		return nil
	}

	eb.SubscribeAll(handler)
	eb.Publish(context.Background(), Event{Type: "test1"})
	eb.Publish(context.Background(), Event{Type: "test2"})

	if count != 2 {
		t.Errorf("expected 2 calls, got %d", count)
	}
}

func TestUnsubscribe(t *testing.T) {
	eb := New()
	called := false
	handler := func(ctx context.Context, event Event) error {
		called = true
		return nil
	}

	eb.Subscribe("test", handler)
	eb.Unsubscribe("test")
	eb.Publish(context.Background(), Event{Type: "test"})

	if called {
		t.Error("handler was called after unsubscribe")
	}
}

func TestPublish(t *testing.T) {
	eb := New()
	received := Event{}
	handler := func(ctx context.Context, event Event) error {
		received = event
		return nil
	}

	eb.Subscribe("test", handler)
	event := Event{Type: "test", Source: "unit-test", Data: "hello"}
	err := eb.Publish(context.Background(), event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if received.Type != "test" {
		t.Errorf("expected type 'test', got '%s'", received.Type)
	}
}

func TestPublish_Error(t *testing.T) {
	eb := New()
	expectedErr := errors.New("handler error")
	handler := func(ctx context.Context, event Event) error {
		return expectedErr
	}

	eb.Subscribe("test", handler)
	err := eb.Publish(context.Background(), Event{Type: "test"})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestPublishAsync(t *testing.T) {
	eb := New()
	called := false
	handler := func(ctx context.Context, event Event) error {
		called = true
		return nil
	}

	eb.Subscribe("test", handler)
	eb.PublishAsync(context.Background(), Event{Type: "test"})

	// Wait for goroutine
	time.Sleep(10 * time.Millisecond)

	if !called {
		t.Error("handler was not called asynchronously")
	}
}

func TestChannel(t *testing.T) {
	eb := New()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ch := eb.Channel(ctx, "test")

	go func() {
		eb.Publish(ctx, Event{Type: "test", Data: "hello"})
	}()

	select {
	case event := <-ch:
		if event.Type != "test" {
			t.Errorf("expected type 'test', got '%s'", event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("did not receive event")
	}
}

func TestSetBuffer(t *testing.T) {
	eb := New()
	eb.SetBuffer(200)

	if eb.buffer != 200 {
		t.Errorf("expected buffer 200, got %d", eb.buffer)
	}
}

func TestMultipleHandlers(t *testing.T) {
	eb := New()
	count := 0
	handler := func(ctx context.Context, event Event) error {
		count++
		return nil
	}

	eb.Subscribe("test", handler)
	eb.Subscribe("test", handler)

	eb.Publish(context.Background(), Event{Type: "test"})

	if count != 2 {
		t.Errorf("expected 2 calls, got %d", count)
	}
}

func TestWildcardAndSpecific(t *testing.T) {
	eb := New()
	var events []string
	specific := func(ctx context.Context, event Event) error {
		events = append(events, "specific")
		return nil
	}
	wildcard := func(ctx context.Context, event Event) error {
		events = append(events, "wildcard")
		return nil
	}

	eb.Subscribe("test", specific)
	eb.SubscribeAll(wildcard)

	eb.Publish(context.Background(), Event{Type: "test"})

	if len(events) != 2 {
		t.Errorf("expected 2 calls, got %d", len(events))
	}
}
