// Package detect runs YOLOv8 object detection on JPEG frames using ONNX Runtime.
package detect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
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

// Detector runs YOLOv8 inference. Process is safe for concurrent callers —
// if inference is already running a frame is passed through unchanged (no queueing).
type Detector struct {
	cfg          Config
	session      *ort.AdvancedSession
	inputTensor  *ort.Tensor[float32]
	outputTensor *ort.Tensor[float32]
	numBoxes     int
	mu           sync.Mutex
	bufPool      sync.Pool
}

var ortOnce sync.Once

// New initialises the ONNX Runtime environment (once per process) and loads the model.
func New(cfg Config) (*Detector, error) {
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

	numBoxes := (cfg.InputWidth/8)*(cfg.InputHeight/8) +
		(cfg.InputWidth/16)*(cfg.InputHeight/16) +
		(cfg.InputWidth/32)*(cfg.InputHeight/32)

	inputShape := ort.NewShape(1, 3, int64(cfg.InputHeight), int64(cfg.InputWidth))
	inputTensor, err := ort.NewEmptyTensor[float32](inputShape)
	if err != nil {
		return nil, fmt.Errorf("input tensor: %w", err)
	}

	outputShape := ort.NewShape(1, int64(4+cfg.NumClasses), int64(numBoxes))
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
		numBoxes:     numBoxes,
		bufPool:      sync.Pool{New: func() any { return new(bytes.Buffer) }},
	}, nil
}

// Destroy releases ORT resources. Call once when the detector is no longer needed.
func (d *Detector) Destroy() {
	d.session.Destroy()
	d.inputTensor.Destroy()
	d.outputTensor.Destroy()
}

// Process implements restream.Processor. If inference is already running for
// another frame, the original jpeg is passed through unchanged.
func (d *Detector) Process(jpeg []byte, push func([]byte)) {
	if !d.mu.TryLock() {
		push(jpeg)
		return
	}
	defer d.mu.Unlock()
	push(d.process(jpeg))
}

func (d *Detector) process(jpeg []byte) []byte {
	src, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil {
		return jpeg
	}

	origW := src.Bounds().Dx()
	origH := src.Bounds().Dy()

	lb := letterbox(src, d.cfg.InputWidth, d.cfg.InputHeight)
	toTensor(lb.img, d.inputTensor.GetData())

	if err := d.session.Run(); err != nil {
		return jpeg
	}

	dets := parseOutput(
		d.outputTensor.GetData(),
		d.cfg.NumClasses, d.numBoxes,
		d.cfg.ConfThreshold, d.cfg.NMSThreshold,
		lb, origW, origH,
	)

	var nrgba *image.NRGBA
	if n, ok := src.(*image.NRGBA); ok {
		nrgba = n
	} else {
		nrgba = image.NewNRGBA(src.Bounds())
		for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
			for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
				nrgba.Set(x, y, src.At(x, y))
			}
		}
	}

	annotated := Annotate(nrgba, dets)

	buf := d.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer d.bufPool.Put(buf)

	if err := imaging.Encode(buf, annotated, imaging.JPEG, imaging.JPEGQuality(d.cfg.JPEGQuality)); err != nil {
		return jpeg
	}

	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out
}
