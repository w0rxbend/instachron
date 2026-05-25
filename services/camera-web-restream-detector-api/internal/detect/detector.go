// Package detect runs YOLOv8 object detection on JPEG frames using ONNX Runtime.
package detect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disintegration/imaging"
	ort "github.com/yalue/onnxruntime_go"
)

// Config holds detector parameters loaded from config.json.
type Config struct {
	ModelPath      string   `json:"model_path"`
	OrtLibPath     string   `json:"ort_lib_path"`    // path to libonnxruntime.so; "" = default "onnxruntime.so"
	InputName      string   `json:"input_name"`      // default "images"
	OutputName     string   `json:"output_name"`     // default "output0"
	ConfThreshold  float32  `json:"conf_threshold"`  // default 0.25
	NMSThreshold   float32  `json:"nms_threshold"`   // default 0.45
	InputWidth     int      `json:"input_width"`     // default 640
	InputHeight    int      `json:"input_height"`    // default 640
	NumClasses     int      `json:"num_classes"`     // default 80
	JPEGQuality    int      `json:"jpeg_quality"`    // default 85
	AllowedClasses []string `json:"allowed_classes"` // if non-empty, only these class names are kept
	Debug          bool     `json:"debug"`
}

func DefaultConfig() Config {
	return Config{
		ModelPath:     "models/yolov8n.onnx",
		OrtLibPath:    "libonnxruntime.so",
		InputName:     "images",
		OutputName:    "output0",
		ConfThreshold: 0.25,
		NMSThreshold:  0.45,
		InputWidth:    640,
		InputHeight:   640,
		NumClasses:    80,
		JPEGQuality:   85,
	}
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// OutputLayout describes how the model's flat output array is indexed.
type OutputLayout struct {
	NumBoxes    int
	NumChannels int
	Transposed  bool // true = [1, numBoxes, numChannels]; false = [1, numChannels, numBoxes]
}

// Detector runs YOLOv8 inference. Process is safe for concurrent callers.
// While inference is in progress, the last annotated frame is served instead
// of the raw input, so the stream always shows the most recent detections.
type Detector struct {
	cfg          Config
	session      *ort.AdvancedSession
	inputTensor  *ort.Tensor[float32]
	outputTensor *ort.Tensor[float32]
	layout       OutputLayout
	allowedIDs   map[int]struct{} // nil = all classes allowed
	logger       *log.Logger
	mu           sync.Mutex
	bufPool      sync.Pool

	lastAnnot atomic.Pointer[[]byte] // most recent annotated JPEG

	// periodic stats (logged every ~10s)
	frameIn  atomic.Int64
	frameDet atomic.Int64
	frameDrp atomic.Int64
	lastLog  atomic.Int64 // UnixNano of last stats log
}

var ortOnce sync.Once

// New initialises the ONNX Runtime environment (once per process) and loads the model.
// logger is used for debug output when cfg.Debug is true; pass nil to use the default logger.
func New(cfg Config, logger *log.Logger) (*Detector, error) {
	if logger == nil {
		logger = log.Default()
	}
	var initErr error
	ortOnce.Do(func() {
		if cfg.OrtLibPath != "" {
			ort.SetSharedLibraryPath(cfg.OrtLibPath)
		}
		initErr = ort.InitializeEnvironment(ort.WithLogLevelError())
	})
	if initErr != nil {
		return nil, fmt.Errorf("ort init: %w", initErr)
	}

	// Probe the model to discover actual tensor names and output shape.
	// This handles any YOLOv8 export variant without manual name configuration.
	probe, err := probeModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("probe model: %w", err)
	}
	layout := probe.Layout
	logger.Printf("model probe: input=%q output=%q shape=%+v", probe.InputName, probe.OutputName, layout)

	inputShape := ort.NewShape(1, 3, int64(cfg.InputHeight), int64(cfg.InputWidth))
	inputTensor, err := ort.NewEmptyTensor[float32](inputShape)
	if err != nil {
		return nil, fmt.Errorf("input tensor: %w", err)
	}

	var outputShape ort.Shape
	if layout.Transposed {
		outputShape = ort.NewShape(1, int64(layout.NumBoxes), int64(layout.NumChannels))
	} else {
		outputShape = ort.NewShape(1, int64(layout.NumChannels), int64(layout.NumBoxes))
	}
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		inputTensor.Destroy()
		return nil, fmt.Errorf("output tensor: %w", err)
	}

	session, err := ort.NewAdvancedSession(
		cfg.ModelPath,
		[]string{probe.InputName},
		[]string{probe.OutputName},
		[]ort.Value{inputTensor},
		[]ort.Value{outputTensor},
		nil,
	)
	if err != nil {
		inputTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("session: %w", err)
	}

	var allowedIDs map[int]struct{}
	if len(cfg.AllowedClasses) > 0 {
		allowedIDs = make(map[int]struct{}, len(cfg.AllowedClasses))
		nameToID := make(map[string]int, len(CocoClasses))
		for i, name := range CocoClasses {
			nameToID[name] = i
		}
		for _, name := range cfg.AllowedClasses {
			if id, ok := nameToID[name]; ok {
				allowedIDs[id] = struct{}{}
			} else {
				logger.Printf("allowed_classes: unknown class name %q (ignored)", name)
			}
		}
		logger.Printf("class filter: only showing %v", cfg.AllowedClasses)
	}

	d := &Detector{
		cfg:          cfg,
		session:      session,
		inputTensor:  inputTensor,
		outputTensor: outputTensor,
		layout:       layout,
		allowedIDs:   allowedIDs,
		logger:       logger,
		bufPool:      sync.Pool{New: func() any { return new(bytes.Buffer) }},
	}
	d.lastLog.Store(time.Now().UnixNano())
	return d, nil
}

type modelProbe struct {
	InputName  string
	OutputName string
	Layout     OutputLayout
}

// probeModel inspects the ONNX model file to discover the actual input/output
// tensor names and output shape, so the session works regardless of export naming.
func probeModel(cfg Config) (modelProbe, error) {
	inputs, outputs, err := ort.GetInputOutputInfo(cfg.ModelPath)
	if err != nil {
		return modelProbe{}, fmt.Errorf("GetInputOutputInfo: %w", err)
	}
	if len(inputs) == 0 {
		return modelProbe{}, fmt.Errorf("model has no inputs")
	}
	if len(outputs) == 0 {
		return modelProbe{}, fmt.Errorf("model has no outputs")
	}

	inputName := inputs[0].Name
	outputName := outputs[0].Name

	// Override with config values if explicitly set (non-default)
	if cfg.InputName != "" && cfg.InputName != "images" {
		inputName = cfg.InputName
	}
	if cfg.OutputName != "" && cfg.OutputName != "output0" {
		outputName = cfg.OutputName
	}

	dims := outputs[0].Dimensions
	if len(dims) != 3 {
		return modelProbe{}, fmt.Errorf("expected 3-D output, got %v", dims)
	}
	// dims = [batch, A, B]; batch is always 1
	a, b := int(dims[1]), int(dims[2])
	numChannels := 4 + cfg.NumClasses

	var layout OutputLayout
	switch {
	case a == numChannels: // [1, 84, 8400] — channel-first
		layout = OutputLayout{NumBoxes: b, NumChannels: a, Transposed: false}
	case b == numChannels: // [1, 8400, 84] — transposed / boxes-first
		layout = OutputLayout{NumBoxes: a, NumChannels: b, Transposed: true}
	default:
		return modelProbe{}, fmt.Errorf(
			"output shape %v doesn't match expected numChannels=%d", dims, numChannels)
	}

	return modelProbe{InputName: inputName, OutputName: outputName, Layout: layout}, nil
}

// Layout returns the probed output layout (useful for logging).
func (d *Detector) Layout() OutputLayout { return d.layout }

// Destroy releases ORT resources. Call once when the detector is no longer needed.
func (d *Detector) Destroy() {
	d.session.Destroy()
	d.inputTensor.Destroy()
	d.outputTensor.Destroy()
}

// Detect runs inference on jpeg, returning the detections and the annotated JPEG.
// It blocks until inference completes (unlike Process which drops busy frames).
func (d *Detector) Detect(jpeg []byte) ([]Detection, []byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.infer(jpeg)
}

// Process implements restream.Processor.
// When inference is already running the last annotated frame is served instead
// of the raw input so viewers always see the most recent bounding boxes.
func (d *Detector) Process(jpeg []byte, push func([]byte)) {
	d.frameIn.Add(1)

	if !d.mu.TryLock() {
		d.frameDrp.Add(1)
		if last := d.lastAnnot.Load(); last != nil {
			push(*last)
		} else {
			push(jpeg)
		}
		d.logStats()
		return
	}
	defer d.mu.Unlock()

	dets, annotated, err := d.infer(jpeg)
	if err != nil {
		d.logger.Printf("detect: inference error: %v", err)
		push(jpeg)
		d.logStats()
		return
	}
	if annotated == nil {
		push(jpeg)
		d.logStats()
		return
	}
	if len(dets) > 0 {
		d.frameDet.Add(1)
	}
	d.lastAnnot.Store(&annotated)
	push(annotated)
	d.logStats()
}

// logStats prints a summary line every 10 seconds.
func (d *Detector) logStats() {
	now := time.Now().UnixNano()
	last := d.lastLog.Load()
	if now-last < int64(10*time.Second) {
		return
	}
	if !d.lastLog.CompareAndSwap(last, now) {
		return
	}
	in := d.frameIn.Load()
	det := d.frameDet.Load()
	drp := d.frameDrp.Load()
	d.logger.Printf("detect stats: %d frames in, %d with detections, %d deferred to cache (conf_threshold=%.2f)",
		in, det, drp, d.cfg.ConfThreshold)
}

// infer is the internal pipeline; caller must hold d.mu.
func (d *Detector) infer(jpeg []byte) ([]Detection, []byte, error) {
	src, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil {
		return nil, nil, fmt.Errorf("decode: %w", err)
	}

	lb := letterbox(src, d.cfg.InputWidth, d.cfg.InputHeight)
	toTensor(lb.img, d.inputTensor.GetData())

	if err := d.session.Run(); err != nil {
		return nil, nil, fmt.Errorf("inference: %w", err)
	}

	raw := d.outputTensor.GetData()

	if d.cfg.Debug {
		maxScore := float32(0)
		for _, v := range raw {
			if v > maxScore {
				maxScore = v
			}
		}
		d.logger.Printf("detect debug: output max_value=%.4f layout=%+v", maxScore, d.layout)
	}

	dets := parseOutput(raw, d.layout, d.cfg.ConfThreshold, d.cfg.NMSThreshold,
		lb, src.Bounds().Dx(), src.Bounds().Dy())

	if d.allowedIDs != nil {
		filtered := dets[:0]
		for _, det := range dets {
			if _, ok := d.allowedIDs[det.ClassID]; ok {
				filtered = append(filtered, det)
			}
		}
		dets = filtered
	}

	if d.cfg.Debug {
		d.logger.Printf("detect debug: %d detection(s) (conf_threshold=%.2f)", len(dets), d.cfg.ConfThreshold)
		for _, det := range dets {
			d.logger.Printf("  [%s] conf=%.3f box=(%.0f,%.0f)-(%.0f,%.0f)",
				det.ClassName, det.Confidence, det.X1, det.Y1, det.X2, det.Y2)
		}
	}

	nrgba := toNRGBA(src)
	annotated := Annotate(nrgba, dets)

	buf := d.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer d.bufPool.Put(buf)

	if err := imaging.Encode(buf, annotated, imaging.JPEG, imaging.JPEGQuality(d.cfg.JPEGQuality)); err != nil {
		return dets, nil, fmt.Errorf("encode: %w", err)
	}

	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return dets, out, nil
}

func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok {
		return n
	}
	dst := image.NewNRGBA(src.Bounds())
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
	return dst
}
