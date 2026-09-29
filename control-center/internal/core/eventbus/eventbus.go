package eventbus

import (
	"context"
	"sync"
)

// Event represents an event in the system
type Event struct {
	Type      string      `json:"type"`
	Source    string      `json:"source"` // plugin name or "system"
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// Handler handles events
type Handler func(ctx context.Context, event Event) error

// EventBus provides pub/sub functionality
type EventBus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	buffer   int
}

// New creates a new event bus
func New() *EventBus {
	return &EventBus{
		handlers: make(map[string][]Handler),
		buffer:   100,
	}
}

// Subscribe registers a handler for an event type
func (eb *EventBus) Subscribe(eventType string, handler Handler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

// SubscribeAll registers a handler for all events
func (eb *EventBus) SubscribeAll(handler Handler) {
	eb.Subscribe("*", handler)
}

// Unsubscribe removes all handlers for an event type
func (eb *EventBus) Unsubscribe(eventType string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	delete(eb.handlers, eventType)
}

// Publish publishes an event to all subscribers
func (eb *EventBus) Publish(ctx context.Context, event Event) error {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	// Get handlers for specific event type
	handlers := eb.handlers[event.Type]

	// Also notify wildcard subscribers
	if event.Type != "*" {
		handlers = append(handlers, eb.handlers["*"]...)
	}

	var lastErr error
	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// PublishAsync publishes an event asynchronously
func (eb *EventBus) PublishAsync(ctx context.Context, event Event) {
	go eb.Publish(ctx, event)
}

// Channel returns a channel that receives events of a specific type
func (eb *EventBus) Channel(ctx context.Context, eventType string) <-chan Event {
	ch := make(chan Event, eb.buffer)

	eb.Subscribe(eventType, func(ctx context.Context, event Event) error {
		select {
		case ch <- event:
		case <-ctx.Done():
		}
		return nil
	})

	return ch
}

// SetBuffer sets the buffer size for channels
func (eb *EventBus) SetBuffer(size int) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.buffer = size
}

// Event types
const (
	// System events
	EventSystemStart    = "system.start"
	EventSystemShutdown = "system.shutdown"

	// Plugin events
	EventPluginRegistered   = "plugin.registered"
	EventPluginUnregistered = "plugin.unregistered"
	EventPluginStarted      = "plugin.started"
	EventPluginStopped      = "plugin.stopped"
	EventPluginError        = "plugin.error"

	// Node events
	EventNodeAdded   = "node.added"
	EventNodeRemoved = "node.removed"
	EventNodeUpdated = "node.updated"
	EventNodeStatus  = "node.status"

	// Task events
	EventTaskCreated = "task.created"
	EventTaskStarted = "task.started"
	EventTaskDone    = "task.done"
	EventTaskFailed  = "task.failed"

	// User events
	EventUserLogin  = "user.login"
	EventUserLogout = "user.logout"

	// Ansible events
	EventPlaybookStart = "ansible.playbook.start"
	EventPlaybookDone  = "ansible.playbook.done"
	EventPlaybookError = "ansible.playbook.error"
	EventTaskOutput    = "ansible.task.output"
)
