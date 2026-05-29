package recorder

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/w0rxbend/instachron/services/camera-recorder/internal/encoder"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/metrics"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/storage"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

type CameraRecorder struct {
	id      uint32
	idText  string
	cfg     Config
	store   storage.Store
	metrics *metrics.Metrics
	logger  *log.Logger

	ctx    context.Context
	cancel context.CancelFunc
	queue  chan streamproto.Frame
	wg     sync.WaitGroup
}

type activeSegment struct {
	pending      *storage.PendingSegment
	encoder      *encoder.FFmpeg
	startedAt    time.Time
	lastRecorded time.Time
}

func NewCameraRecorder(parent context.Context, id uint32, cfg Config, store storage.Store, m *metrics.Metrics, logger *log.Logger) *CameraRecorder {
	ctx, cancel := context.WithCancel(parent)
	c := &CameraRecorder{
		id:      id,
		idText:  fmt.Sprintf("%d", id),
		cfg:     cfg,
		store:   store,
		metrics: m,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
		queue:   make(chan streamproto.Frame, cfg.QueueSizePerCamera),
	}
	c.wg.Add(1)
	go c.run()
	return c
}

func (c *CameraRecorder) Submit(f streamproto.Frame) {
	c.metrics.IncFramesReceived(c.idText)
	select {
	case c.queue <- f:
	default:
		select {
		case <-c.queue:
			c.metrics.IncFramesDropped(c.idText)
		default:
		}
		select {
		case c.queue <- f:
		default:
			c.metrics.IncFramesDropped(c.idText)
		}
	}
}

func (c *CameraRecorder) Close() {
	c.cancel()
	c.wg.Wait()
}

func (c *CameraRecorder) run() {
	defer c.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	keepEvery := time.Second * time.Duration(c.cfg.TimelapseFactor) / time.Duration(c.cfg.OutputFPS)
	var nextKeepAt time.Time
	var lastFrameAt time.Time
	var seg *activeSegment

	for {
		select {
		case <-c.ctx.Done():
			c.closeSegment(seg, false)
			return
		case <-ticker.C:
			if seg != nil && !lastFrameAt.IsZero() && time.Since(lastFrameAt) > c.cfg.InactiveCloseDuration {
				c.logger.Printf("camera-recorder camera=%s closing inactive segment", c.idText)
				c.closeSegment(seg, false)
				seg = nil
			}
		case f := <-c.queue:
			ts := f.Timestamp
			if ts.IsZero() {
				ts = time.Now()
			}
			lastFrameAt = ts
			if !looksLikeJPEG(f.Payload) {
				c.metrics.IncFramesDropped(c.idText)
				continue
			}
			if !shouldKeep(ts, &nextKeepAt, keepEvery) {
				c.metrics.IncFramesDropped(c.idText)
				continue
			}
			if seg != nil && c.shouldRotate(seg, ts) {
				c.closeSegment(seg, false)
				seg = nil
			}
			if seg == nil {
				var err error
				seg, err = c.openSegment(ts)
				if err != nil {
					c.metrics.IncEncoderError()
					c.logger.Printf("camera-recorder camera=%s open segment: %v", c.idText, err)
					continue
				}
			}
			if err := seg.encoder.WriteJPEG(f.Payload); err != nil {
				c.metrics.IncEncoderError()
				c.logger.Printf("camera-recorder camera=%s write frame: %v", c.idText, err)
				c.closeSegment(seg, true)
				seg = nil
				continue
			}
			seg.lastRecorded = ts
			c.metrics.IncFramesRecorded(c.idText)
			if c.shouldRotate(seg, ts) {
				c.closeSegment(seg, false)
				seg = nil
			}
		}
	}
}

func shouldKeep(ts time.Time, next *time.Time, interval time.Duration) bool {
	if next.IsZero() {
		*next = ts.Add(interval)
		return true
	}
	if ts.Before(*next) {
		return false
	}
	for !next.After(ts) {
		*next = next.Add(interval)
	}
	return true
}

func (c *CameraRecorder) shouldRotate(seg *activeSegment, now time.Time) bool {
	if seg == nil {
		return false
	}
	if now.Sub(seg.startedAt) >= c.cfg.SegmentRawDuration {
		return true
	}
	return seg.pending.Writer.BytesWritten() >= c.cfg.MaxFileBytes
}

func (c *CameraRecorder) openSegment(start time.Time) (*activeSegment, error) {
	pending, err := c.store.BeginSegment(c.ctx, c.idText, start, c.cfg.OutputFPS, c.cfg.TimelapseFactor)
	if err != nil {
		return nil, err
	}
	encCfg := c.cfg.FFmpeg
	encCfg.OutputFPS = c.cfg.OutputFPS
	enc, err := encoder.Start(context.Background(), encCfg, pending.Writer)
	if err != nil {
		_ = c.store.DiscardSegment(context.Background(), pending)
		return nil, err
	}
	c.metrics.IncActiveEncoders()
	c.logger.Printf("camera-recorder camera=%s started segment %s", c.idText, pending.Info.FileName)
	return &activeSegment{pending: pending, encoder: enc, startedAt: start, lastRecorded: start}, nil
}

func (c *CameraRecorder) closeSegment(seg *activeSegment, discard bool) {
	if seg == nil {
		return
	}
	c.metrics.DecActiveEncoders()
	if discard {
		seg.encoder.Kill()
		_ = c.store.DiscardSegment(context.Background(), seg.pending)
		return
	}
	if err := seg.encoder.Close(); err != nil {
		c.metrics.IncEncoderError()
		c.logger.Printf("camera-recorder camera=%s finalize encoder: %v", c.idText, err)
		_ = c.store.DiscardSegment(context.Background(), seg.pending)
		return
	}
	if err := seg.pending.Writer.Close(); err != nil {
		c.metrics.IncEncoderError()
		c.logger.Printf("camera-recorder camera=%s close segment writer: %v", c.idText, err)
		_ = c.store.DiscardSegment(context.Background(), seg.pending)
		return
	}
	info, err := c.store.CompleteSegment(context.Background(), seg.pending, seg.lastRecorded)
	if err != nil {
		c.metrics.IncEncoderError()
		c.logger.Printf("camera-recorder camera=%s complete segment: %v", c.idText, err)
		_ = c.store.DiscardSegment(context.Background(), seg.pending)
		return
	}
	c.metrics.IncSegmentCompleted(c.idText)
	if err := c.store.Prune(context.Background(), c.idText, c.cfg.KeepFilesPerCamera); err != nil {
		c.logger.Printf("camera-recorder camera=%s prune: %v", c.idText, err)
	}
	c.logger.Printf("camera-recorder camera=%s completed %s size=%d", c.idText, info.FileName, info.SizeBytes)
}

func looksLikeJPEG(b []byte) bool {
	return len(b) >= 4 && b[0] == 0xFF && b[1] == 0xD8 && b[len(b)-2] == 0xFF && b[len(b)-1] == 0xD9
}
