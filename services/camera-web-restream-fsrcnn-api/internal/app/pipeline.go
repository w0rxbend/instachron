package app

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/disintegration/imaging"
)

// processJob is a single frame to be upscaled. pushFn delivers the result to
// the camera hub without the pipeline needing to know about hub internals.
type processJob struct {
	jpeg   []byte
	pushFn func([]byte)
}

// workerPool distributes FSRCNN upscaling across a fixed set of goroutines,
// each owning its own ONNX session. The bounded queue drops the oldest pending
// frame when full, keeping latency bounded at the cost of dropped frames.
type workerPool struct {
	queue   chan processJob
	wg      sync.WaitGroup
	metrics *pipelineMetrics
}

// newWorkerPool starts worker goroutines and returns a ready pool.
// Each session in sessions is owned exclusively by one goroutine.
func newWorkerPool(sessions []*fsrcnnSession, queueSize, jpegQuality, maxW, maxH, scale int, m *pipelineMetrics) *workerPool {
	p := &workerPool{
		queue:   make(chan processJob, queueSize),
		metrics: m,
	}
	for _, sess := range sessions {
		sess := sess
		p.wg.Add(1)
		go p.workerLoop(sess, jpegQuality, maxW, maxH, scale)
	}
	return p
}

// submit enqueues a frame. When the queue is full the oldest pending frame is
// evicted to make room (drop-oldest policy).
func (p *workerPool) submit(jpeg []byte, pushFn func([]byte)) {
	j := processJob{jpeg: jpeg, pushFn: pushFn}
	select {
	case p.queue <- j:
	default:
		// Queue full: drain oldest then insert new.
		select {
		case <-p.queue:
			p.metrics.recordDrop()
		default:
		}
		select {
		case p.queue <- j:
		default:
			// Concurrent submitter won the slot; drop the incoming frame.
			p.metrics.recordDrop()
		}
	}
}

// close drains remaining jobs, waits for all workers to finish, and destroys
// their ONNX sessions.
func (p *workerPool) close() {
	close(p.queue)
	p.wg.Wait()
}

func (p *workerPool) workerLoop(sess *fsrcnnSession, jpegQuality, maxW, maxH, scale int) {
	defer p.wg.Done()
	defer sess.destroy()

	var buf bytes.Buffer

	for j := range p.queue {
		out := p.process(sess, j.jpeg, &buf, jpegQuality, maxW, maxH, scale)
		if out != nil {
			j.pushFn(out)
		}
	}
}

func (p *workerPool) process(
	sess *fsrcnnSession,
	jpeg []byte,
	buf *bytes.Buffer,
	jpegQuality, maxW, maxH, scale int,
) []byte {
	t0 := time.Now()

	// --- decode ---
	img, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil {
		return nil
	}
	tDecode := time.Since(t0)

	// --- preprocess ---
	t1 := time.Now()
	img = capResolution(img, maxW, maxH)
	yData, cbImg, crImg, imgW, imgH := splitYCbCr(img)
	tPreprocess := time.Since(t1)

	// --- FSRCNN inference (Y channel only) ---
	t2 := time.Now()
	yUp, err := sess.run(yData, imgH, imgW)
	if err != nil {
		return nil
	}
	clampY(yUp)
	tInference := time.Since(t2)

	// --- postprocess: upscale Cb/Cr + merge ---
	t3 := time.Now()
	outW, outH := imgW*scale, imgH*scale
	cbUp := imaging.Resize(cbImg, outW, outH, imaging.CatmullRom)
	crUp := imaging.Resize(crImg, outW, outH, imaging.CatmullRom)
	merged := mergeYCbCr(yUp, cbUp, crUp, outW, outH)
	tPostprocess := time.Since(t3)

	// --- encode ---
	t4 := time.Now()
	out, err := encodeJPEG(merged, jpegQuality, buf)
	if err != nil {
		return nil
	}
	tEncode := time.Since(t4)

	p.metrics.record(tDecode, tPreprocess, tInference, tPostprocess, tEncode, time.Since(t0))
	return out
}

// warmup runs a small dummy inference through every session in the pool to
// pre-compile ORT kernels and stabilise latency before real traffic arrives.
func warmupSessions(sessions []*fsrcnnSession, scale, runs int) {
	const dummyW, dummyH = 160, 90
	dummy := make([]float32, dummyW*dummyH)
	var wg sync.WaitGroup
	for _, sess := range sessions {
		sess := sess
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < runs; i++ {
				sess.run(dummy, dummyH, dummyW)
			}
		}()
	}
	wg.Wait()
}

// runMetricsReporter starts the periodic metrics logger.
func runMetricsReporter(ctx context.Context, m *pipelineMetrics, cfg *appConfig) {
	m.runReporter(ctx, cfg.metricsInterval, cfg.logger)
}
