// Package pipeline distributes Lanczos upscaling across a fixed worker pool.
package pipeline

import (
	"bytes"
	"image"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/w0rxbend/instachron/services/camera-web-restream-fsrcnn-api/internal/imageio"
	"github.com/w0rxbend/instachron/services/camera-web-restream-fsrcnn-api/internal/metrics"
)

type job struct {
	jpeg   []byte
	pushFn func([]byte)
}

// Pool distributes Lanczos upscaling across a fixed set of goroutines.
// The bounded queue drops the oldest pending frame when full to keep latency
// bounded at the cost of dropped frames under sustained overload.
// Pool implements restream.Processor via Process.
type Pool struct {
	queue       chan job
	wg          sync.WaitGroup
	metrics     *metrics.Pipeline
	jpegQuality int
	maxW, maxH  int
	scale       int
}

// New starts numWorkers goroutines and returns a ready Pool.
// scale is the integer upscale factor (e.g. 2 for 2×). Factors that are a
// power of two are decomposed into sequential 2× passes to avoid the soft
// artifacts of a single large upscale.
func New(numWorkers, queueSize, jpegQuality, maxW, maxH, scale int, m *metrics.Pipeline) *Pool {
	p := &Pool{
		queue:       make(chan job, queueSize),
		metrics:     m,
		jpegQuality: jpegQuality,
		maxW:        maxW,
		maxH:        maxH,
		scale:       scale,
	}
	for range numWorkers {
		p.wg.Add(1)
		go p.workerLoop()
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

// Close drains remaining jobs and waits for all workers to finish.
func (p *Pool) Close() {
	close(p.queue)
	p.wg.Wait()
}

func (p *Pool) workerLoop() {
	defer p.wg.Done()
	var buf bytes.Buffer
	for j := range p.queue {
		out := p.process(j.jpeg, &buf)
		if out != nil {
			j.pushFn(out)
		}
	}
}

func (p *Pool) process(jpeg []byte, buf *bytes.Buffer) []byte {
	t0 := time.Now()

	src, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil {
		return nil
	}
	tDecode := time.Since(t0)

	t1 := time.Now()
	src = imageio.CapResolution(src, p.maxW, p.maxH)
	// Mild denoise before upscaling to suppress JPEG block noise.
	src = imaging.Blur(src, 0.3)
	dst := upscale(src, p.scale)
	tResize := time.Since(t1)

	t2 := time.Now()
	out, err := imageio.EncodeJPEG(dst, p.jpegQuality, buf)
	if err != nil {
		return nil
	}
	tEncode := time.Since(t2)

	p.metrics.Record(tDecode, tResize, tEncode, time.Since(t0))
	return out
}

// upscale applies the best-quality strategy for the given integer scale factor.
// Powers of two are broken into sequential 2× steps to avoid the soft
// artifacts of a single large resize. Each 2× step applies Lanczos then a
// moderate sharpen; a final contrast nudge is applied once at the end.
func upscale(src image.Image, scale int) image.Image {
	passes := stepsFor(scale)
	for i, s := range passes {
		b := src.Bounds()
		src = imaging.Resize(src, b.Dx()*s, b.Dy()*s, imaging.Lanczos)
		if i < len(passes)-1 {
			// Intermediate pass: lighter sharpen to avoid compounding halos.
			src = imaging.Sharpen(src, 0.4)
		} else {
			src = imaging.Sharpen(src, 0.8)
		}
	}
	return imaging.AdjustContrast(src, 3)
}

// stepsFor decomposes scale into a sequence of 2× passes where possible,
// falling back to a single step for scales that are not powers of two.
func stepsFor(scale int) []int {
	if scale <= 1 {
		return nil
	}
	// Decompose power-of-two scales into 2× passes.
	if scale&(scale-1) == 0 {
		steps := make([]int, 0, 4)
		for scale > 1 {
			steps = append(steps, 2)
			scale >>= 1
		}
		return steps
	}
	return []int{scale}
}
