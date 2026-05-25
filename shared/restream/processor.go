package restream

// Processor transforms a raw JPEG frame before it is published.
// Process must call push exactly once (synchronously or asynchronously) and
// must be safe for concurrent use.
type Processor interface {
	Process(jpeg []byte, push func([]byte))
}

// Noop is a Processor that delivers frames unchanged.
type Noop struct{}

func (Noop) Process(jpeg []byte, push func([]byte)) { push(jpeg) }
