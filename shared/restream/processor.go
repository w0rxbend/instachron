package restream

// Processor transforms a raw JPEG frame and delivers it to a hub.
// Process is called by upstreamReader for each received frame.
// Implementations must call push at some point (synchronously or asynchronously)
// and must be safe for concurrent use across multiple upstream goroutines.
type Processor interface {
	Process(jpeg []byte, push func([]byte))
}

// Noop is a Processor that delivers frames unchanged.
type Noop struct{}

func (Noop) Process(jpeg []byte, push func([]byte)) { push(jpeg) }
