package restream

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

// TCPUpstreamConfig controls connection and retry behaviour.
type TCPUpstreamConfig struct {
	Addr       string
	MinBackoff time.Duration
	MaxBackoff time.Duration
}

// TCPUpstream connects to an upstream TCP frame server, reads streamproto
// frames, and calls onFrame synchronously for each received frame.
// Reconnection uses exponential backoff. onOffline fires whenever the
// connection drops; it may be nil.
type TCPUpstream struct {
	cfg       TCPUpstreamConfig
	onFrame   func(streamproto.Frame)
	onOffline func()
	logger    *log.Logger
}

// NewTCPUpstream creates a TCPUpstream.
// onFrame is called in the read loop and must not block for extended periods;
// use a goroutine internally if processing is heavy.
func NewTCPUpstream(
	cfg TCPUpstreamConfig,
	onFrame func(streamproto.Frame),
	onOffline func(),
	logger *log.Logger,
) *TCPUpstream {
	if cfg.MinBackoff == 0 {
		cfg.MinBackoff = 500 * time.Millisecond
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 10 * time.Second
	}
	return &TCPUpstream{cfg: cfg, onFrame: onFrame, onOffline: onOffline, logger: logger}
}

// Run connects and reads frames until ctx is cancelled, reconnecting with
// exponential backoff on every disconnect.
func (u *TCPUpstream) Run(ctx context.Context) {
	backoff := u.cfg.MinBackoff
	for {
		if ctx.Err() != nil {
			return
		}

		frames, err := u.connect(ctx)

		if ctx.Err() != nil {
			return
		}
		if u.onOffline != nil {
			u.onOffline()
		}

		if frames > 0 {
			backoff = u.cfg.MinBackoff
		} else {
			backoff = min(backoff*2, u.cfg.MaxBackoff)
		}
		if err != nil {
			u.logger.Printf("TCP upstream %s: %v (retry in %s)", u.cfg.Addr, err, backoff)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (u *TCPUpstream) connect(ctx context.Context) (int, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", u.cfg.Addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	u.logger.Printf("TCP upstream connected to %s", u.cfg.Addr)

	r := streamproto.NewReader(conn)
	frames := 0
	for {
		f, err := r.ReadFrame(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return frames, nil
			}
			return frames, err
		}
		if u.onFrame != nil {
			u.onFrame(f)
		}
		frames++
	}
}
