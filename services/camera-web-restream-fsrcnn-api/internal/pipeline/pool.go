// Package pipeline distributes FSRCNN upscaling across a fixed worker pool.
// Each worker owns its own ONNX session to avoid concurrent session use.
package pipeline

import (
	"bytes"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/w0rxbend/instachron/services/camera-web-restream-fsrcnn-api/internal/fsrcnn"
	"github.com/w0rxbend/instachron/services/camera-web-restream-fsrcnn-api/internal/imageio"
	"github.com/w0rxbend/instachron/services/camera-web-restream-fsrcnn-api/internal/metrics"
)

type job struct {
	jpeg   []byte
	pushFn func([]byte)
}

// Pool distributes FSRCNN upscaling across a fixed set of goroutines.
// The bounded queue drops the oldest pending frame when full, keeping latency
// bounded at the cost of dropped frames under sustained overload.
// Pool implements restream.Processor via Process.
type Pool struct {
	queue   chan job
	wg      sync.WaitGroup
	metrics *metrics.Pipeline

	jpegQuality int
	maxW, maxH  int
	scale       int
}

// New starts worker goroutines and returns a ready Pool.
// Each session in sessions is owned exclusively by one goroutine.
func New(sessions []*fsrcnn.Session, queueSize, jpegQuality, maxW, maxH, scale int, m *metrics.Pipeline) *Pool {
	p := &Pool{
		queue:       make(chan job, queueSize),
		metrics:     m,
		jpegQuality: jpegQuality,
		maxW:        maxW,
		maxH:        maxH,
		scale:       scale,
	}
	for _, sess := range sessions {
		sess := sess
		p.wg.Add(1)
		go p.workerLoop(sess)
	}
	return p
}

// Process implements restream.Processor. It enqueues jpeg for upscaling and
// arranges for push to be called with the result. Process is non-blocking:
// when the queue is full the oldest pending frame is evicted (drop-oldest).
func (p *Pool) Process(jpeg []byte, push func([]byte)) {
	j := job{jpeg: jpeg, pushFn: push}
	select {
	case p.queue <- j:
	default:
		select {
		case <-p.queue:
			p.metrics.RecordDrop()
		default:
		}
		select {
		case p.queue <- j:
		default:
			p.metrics.RecordDrop()
		}
	}
}

// Close drains remaining jobs, waits for all workers to finish, and destroys
// their ONNX sessions.
func (p *Pool) Close() {
	close(p.queue)
	p.wg.Wait()
}

// Warmup runs dummy inferences through every session to pre-compile ORT kernels.
func Warmup(sessions []*fsrcnn.Session, scale, runs int) {
	const dummyW, dummyH = 160, 90
	dummy := make([]float32, dummyW*dummyH)
	var wg sync.WaitGroup
	for _, sess := range sessions {
		sess := sess
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < runs; i++ {
				sess.Run(dummy, dummyH, dummyW)
			}
		}()
	}
	wg.Wait()
}

func (p *Pool) workerLoop(sess *fsrcnn.Session) {
	defer p.wg.Done()
	defer sess.Destroy()

	var buf bytes.Buffer
	for j := range p.queue {
		out := p.process(sess, j.jpeg, &buf)
		if out != nil {
			j.pushFn(out)
		}
	}
}

func (p *Pool) process(sess *fsrcnn.Session, jpeg []byte, buf *bytes.Buffer) []byte {
	t0 := time.Now()

	img, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil {
		return nil
	}
	tDecode := time.Since(t0)

	t1 := time.Now()
	img = imageio.CapResolution(img, p.maxW, p.maxH)
	yData, cbImg, crImg, imgW, imgH := imageio.SplitYCbCr(img)
	tPreprocess := time.Since(t1)

	t2 := time.Now()
	yUp, err := sess.Run(yData, imgH, imgW)
	if err != nil {
		return nil
	}
	imageio.ClampY(yUp)
	tInference := time.Since(t2)

	t3 := time.Now()
	outW, outH := imgW*p.scale, imgH*p.scale
	cbUp := imaging.Resize(cbImg, outW, outH, imaging.CatmullRom)
	crUp := imaging.Resize(crImg, outW, outH, imaging.CatmullRom)
	merged := imageio.MergeYCbCr(yUp, cbUp, crUp, outW, outH)
	tPostprocess := time.Since(t3)

	t4 := time.Now()
	out, err := imageio.EncodeJPEG(merged, p.jpegQuality, buf)
	if err != nil {
		return nil
	}
	tEncode := time.Since(t4)

	p.metrics.Record(tDecode, tPreprocess, tInference, tPostprocess, tEncode, time.Since(t0))
	return out
}
