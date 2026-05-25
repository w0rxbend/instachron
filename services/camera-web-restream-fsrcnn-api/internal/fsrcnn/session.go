// Package fsrcnn wraps a single ONNX Runtime session for FSRCNN super-resolution.
// One session per worker goroutine — never shared concurrently.
package fsrcnn

import (
	"fmt"

	ort "github.com/yalue/onnxruntime_go"
)

// Session wraps a single ONNX Runtime dynamic session for FSRCNN inference.
type Session struct {
	inner      *ort.DynamicAdvancedSession
	inputName  string
	outputName string
	scale      int
}

// New loads the ONNX model and creates a session with the given thread
// configuration. Must be called after ort.InitializeEnvironment().
func New(modelPath, inputName, outputName string, scale, intraThreads, interThreads int) (*Session, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("session options: %w", err)
	}
	if err := opts.SetIntraOpNumThreads(intraThreads); err != nil {
		opts.Destroy()
		return nil, fmt.Errorf("intra op threads: %w", err)
	}
	if err := opts.SetInterOpNumThreads(interThreads); err != nil {
		opts.Destroy()
		return nil, fmt.Errorf("inter op threads: %w", err)
	}

	sess, err := ort.NewDynamicAdvancedSession(
		modelPath,
		[]string{inputName},
		[]string{outputName},
		opts,
	)
	opts.Destroy()
	if err != nil {
		return nil, fmt.Errorf("create dynamic session: %w", err)
	}

	return &Session{
		inner:      sess,
		inputName:  inputName,
		outputName: outputName,
		scale:      scale,
	}, nil
}

// Run performs FSRCNN super-resolution inference.
// Input: Y channel as float32 [h*w], normalised [0, 1], shape [1,1,h,w].
// Output: upscaled Y as float32 [outH*outW], shape [1,1,h*scale,w*scale].
func (s *Session) Run(yData []float32, h, w int) ([]float32, error) {
	outH := h * s.scale
	outW := w * s.scale

	inTensor, err := ort.NewTensor(ort.NewShape(1, 1, int64(h), int64(w)), yData)
	if err != nil {
		return nil, fmt.Errorf("create input tensor: %w", err)
	}
	defer inTensor.Destroy()

	outData := make([]float32, outH*outW)
	outTensor, err := ort.NewTensor(ort.NewShape(1, 1, int64(outH), int64(outW)), outData)
	if err != nil {
		return nil, fmt.Errorf("create output tensor: %w", err)
	}
	defer outTensor.Destroy()

	if err := s.inner.Run([]ort.Value{inTensor}, []ort.Value{outTensor}); err != nil {
		return nil, fmt.Errorf("inference: %w", err)
	}
	return outData, nil
}

// Destroy releases the ORT session resources.
func (s *Session) Destroy() {
	s.inner.Destroy()
}
