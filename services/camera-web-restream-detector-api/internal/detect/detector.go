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

	"github.com/disintegration/imaging"
	ort "github.com/yalue/onnxruntime_go"
)

// Config holds detector parameters loaded from config.json.
type Config struct {
	ModelPath     string  `json:"model_path"`
	OrtLibPath    string  `json:"ort_lib_path"`   // path to libonnxruntime.so; "" = default "onnxruntime.so"
	InputName     string  `json:"input_name"`     // default "images"
	OutputName    string  `json:"output_name"`    // default "output0"
	ConfThreshold float32 `json:"conf_threshold"` // default 0.5
	NMSThreshold  float32 `json:"nms_threshold"`  // default 0.45
	InputWidth    int     `json:"input_width"`    // default 640
	InputHeight   int     `json:"input_height"`   // default 640
	NumClasses    int     `json:"num_classes"`    // default 80
	JPEGQuality   int     `json:"jpeg_quality"`   // default 85
	Debug         bool    `json:"debug"`          // log max score and detection count per frame
}

func DefaultConfig() Config {
	return Config{
		ModelPath:     "models/yolov8n.onnx",
		OrtLibPath:    "libonnxruntime.so",
		InputName:     "images",
		OutputName:    "output0",
		ConfThreshold: 0.5,
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

// Detector runs YOLOv8 inference. Process is safe for concurrent callers —
// if inference is already running a frame is passed through unchanged (no queueing).
type Detector struct {
	cfg          Config
	session      *ort.AdvancedSession
	inputTensor  *ort.Tensor[float32]
	outputTensor *ort.Tensor[float32]
	layout       OutputLayout
	logger       *log.Logger
	mu           sync.Mutex
	bufPool      sync.Pool
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

	// Query the model's actual output shape so we allocate the tensor correctly
	// and index the data correctly regardless of export format.
	// YOLOv8 can export as [1, 4+classes, boxes] or transposed [1, boxes, 4+classes].
	layout, err := probeOutputLayout(cfg)
	if err != nil {
		return nil, fmt.Errorf("probe model output: %w", err)
	}

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
		[]string{cfg.InputName},
		[]string{cfg.OutputName},
		[]ort.Value{inputTensor},
		[]ort.Value{outputTensor},
		nil,
	)
	if err != nil {
		inputTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("session: %w", err)
	}

	return &Detector{
		cfg:          cfg,
		session:      session,
		inputTensor:  inputTensor,
		outputTensor: outputTensor,
		layout:       layout,
		logger:       logger,
		bufPool:      sync.Pool{New: func() any { return new(bytes.Buffer) }},
	}, nil
}

// probeOutputLayout inspects the model file to determine the actual output tensor
// shape and returns the appropriate indexing layout.
func probeOutputLayout(cfg Config) (OutputLayout, error) {
	_, outputs, err := ort.GetInputOutputInfo(cfg.ModelPath)
	if err != nil {
		return OutputLayout{}, fmt.Errorf("GetInputOutputInfo: %w", err)
	}
	if len(outputs) == 0 {
		return OutputLayout{}, fmt.Errorf("model has no outputs")
	}

	dims := outputs[0].Dimensions
	if len(dims) != 3 {
		return OutputLayout{}, fmt.Errorf("expected 3-D output, got %v", dims)
	}
	// dims = [batch, A, B]; batch is always 1
	a, b := int(dims[1]), int(dims[2])
	numChannels := 4 + cfg.NumClasses

	switch {
	case a == numChannels: // [1, 84, 8400] — channel-first
		return OutputLayout{NumBoxes: b, NumChannels: a, Transposed: false}, nil
	case b == numChannels: // [1, 8400, 84] — transposed / boxes-first
		return OutputLayout{NumBoxes: a, NumChannels: b, Transposed: true}, nil
	default:
		return OutputLayout{}, fmt.Errorf(
			"output shape %v doesn't match expected numChannels=%d", dims, numChannels)
	}
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

// Process implements restream.Processor. If inference is already in progress the
// original frame is passed through unchanged to keep stream latency bounded.
func (d *Detector) Process(jpeg []byte, push func([]byte)) {
	if !d.mu.TryLock() {
		push(jpeg)
		return
	}
	defer d.mu.Unlock()
	_, annotated, err := d.infer(jpeg)
	if err != nil || annotated == nil {
		push(jpeg)
		return
	}
	push(annotated)
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
