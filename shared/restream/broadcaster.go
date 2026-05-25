package restream

import (
	"sync"
	"sync/atomic"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

// Broadcaster fans streamproto frames out to all subscribed channels.
// It stores the latest frame and delivers it immediately to new subscribers.
// Slow subscribers get their oldest buffered frame dropped rather than
// blocking the publish path.
type Broadcaster struct {
	mu     sync.Mutex
	subs   map[chan streamproto.Frame]struct{}
	latest atomic.Pointer[streamproto.Frame]
}

// NewBroadcaster returns an idle Broadcaster with no subscribers.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: make(map[chan streamproto.Frame]struct{})}
}

// Publish stores f as the latest frame and fans it out to all subscribers.
// Channels that are full have their oldest entry drained before the new frame
// is queued, so subscribers always hold the freshest data.
func (b *Broadcaster) Publish(f streamproto.Frame) {
	b.latest.Store(&f)

	b.mu.Lock()
	for ch := range b.subs {
		select {
		case ch <- f:
		default:
			// drain oldest, then push newest
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- f:
			default:
			}
		}
	}
	b.mu.Unlock()
}

// Subscribe registers a new subscriber. The latest known frame is sent
// immediately so the client renders without waiting for the next push.
// The returned unsubscribe function must be called exactly once when done.
func (b *Broadcaster) Subscribe() (<-chan streamproto.Frame, func()) {
	ch := make(chan streamproto.Frame, 2)

	b.mu.Lock()
	b.subs[ch] = struct{}{}
	if p := b.latest.Load(); p != nil {
		select {
		case ch <- *p:
		default:
		}
	}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
		close(ch)
	}
}
