// Package metrics accumulates pipeline timing counters for the FSRCNN service.
package metrics

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// Pipeline accumulates per-operation nanosecond totals and frame counts using
// lock-free atomics. A background goroutine logs rolling averages.
type Pipeline struct {
	frames        atomic.Int64
	dropped       atomic.Int64
	decodeNs      atomic.Int64
	preprocessNs  atomic.Int64
	inferenceNs   atomic.Int64
	postprocessNs atomic.Int64
	encodeNs      atomic.Int64
	totalNs       atomic.Int64
}

// RecordDrop increments the dropped-frame counter.
func (m *Pipeline) RecordDrop() {
	m.dropped.Add(1)
}

// Record adds one frame's timing breakdown to the running totals.
func (m *Pipeline) Record(decode, preprocess, inference, postprocess, encode, total time.Duration) {
	m.frames.Add(1)
	m.decodeNs.Add(int64(decode))
	m.preprocessNs.Add(int64(preprocess))
	m.inferenceNs.Add(int64(inference))
	m.postprocessNs.Add(int64(postprocess))
	m.encodeNs.Add(int64(encode))
	m.totalNs.Add(int64(total))
}

// RunReporter logs aggregate metrics every interval until ctx is done.
func (m *Pipeline) RunReporter(ctx context.Context, interval time.Duration, logger *log.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastFrames, lastDropped int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			frames := m.frames.Load()
			dropped := m.dropped.Load()
			delta := frames - lastFrames
			droppedDelta := dropped - lastDropped
			lastFrames = frames
			lastDropped = dropped

			if delta == 0 {
				continue
			}

			avg := func(ns *atomic.Int64) float64 {
				return float64(ns.Load()) / float64(frames) / 1e6
			}

			logger.Printf(
				"pipeline: frames=%d dropped=%d | avg decode=%.1fms preprocess=%.1fms inference=%.1fms postprocess=%.1fms encode=%.1fms total=%.1fms",
				delta, droppedDelta,
				avg(&m.decodeNs),
				avg(&m.preprocessNs),
				avg(&m.inferenceNs),
				avg(&m.postprocessNs),
				avg(&m.encodeNs),
				avg(&m.totalNs),
			)
		}
	}
}
