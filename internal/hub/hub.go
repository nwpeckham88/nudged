package hub

import (
	"sync"
)

// Message is a simple transport used by the Hub.
type Message struct {
	Topic   string
	Payload any
	From    string
}

// Subscriber receives published Messages.
type Subscriber chan Message

// Hub is an in-memory pub/sub hub suitable for tests and local usage.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[Subscriber]struct{}
}

// New creates a new Hub.
func New() *Hub {
	return &Hub{
		subs: make(map[string]map[Subscriber]struct{}),
	}
}

// Subscribe registers a new subscriber for a topic. It returns the
// subscriber channel and an unsubscribe function.
func (h *Hub) Subscribe(topic string) (Subscriber, func()) {
	ch := make(Subscriber, 16)

	h.mu.Lock()
	defer h.mu.Unlock()
	m, ok := h.subs[topic]
	if !ok {
		m = make(map[Subscriber]struct{})
		h.subs[topic] = m
	}
	m[ch] = struct{}{}

	unsub := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if subs, ok := h.subs[topic]; ok {
			delete(subs, ch)
			close(ch)
			if len(subs) == 0 {
				delete(h.subs, topic)
			}
		}
	}

	return ch, unsub
}

// Publish sends a message to all subscribers of the message's topic.
// Sends are non-blocking; if a subscriber channel is full the message
// to that subscriber is dropped.
func (h *Hub) Publish(msg Message) {
	h.mu.RLock()
	subs := h.subs[msg.Topic]
	// Create a snapshot of subscribers to avoid holding the lock while sending.
	targets := make([]Subscriber, 0, len(subs))
	for s := range subs {
		targets = append(targets, s)
	}
	h.mu.RUnlock()

	for _, s := range targets {
		select {
		case s <- msg:
		default:
			// drop if subscriber not ready
		}
	}
}
