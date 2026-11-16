package server

import "sync"

// subscriberBuffer is how many events a slow WebSocket may lag behind before it misses some.
const subscriberBuffer = 16

// Event is one message on the /api/ws WebSocket.
type Event struct {
	Type    string `json:"type"`
	Version string `json:"version,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

// Progress fans events out to every subscribed WebSocket; later tickets publish to it.
// Publish never blocks: a subscriber whose buffer is full misses the event.
type Progress struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// NewProgress returns a broadcaster with no subscribers.
func NewProgress() *Progress {
	return &Progress{subs: map[chan Event]struct{}{}}
}

// Subscribe registers a receiver; call the returned func to unregister it.
func (p *Progress) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)
	p.mu.Lock()
	p.subs[ch] = struct{}{}
	p.mu.Unlock()
	return ch, func() {
		p.mu.Lock()
		delete(p.subs, ch)
		p.mu.Unlock()
	}
}

// Publish sends ev to every current subscriber.
func (p *Progress) Publish(ev Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for ch := range p.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}
