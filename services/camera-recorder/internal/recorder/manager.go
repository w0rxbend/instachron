package recorder

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/w0rxbend/instachron/services/camera-recorder/internal/encoder"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/metrics"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/storage"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

type Config struct {
	OutputFPS             int
	TimelapseFactor       int
	SegmentRawDuration    time.Duration
	MaxFileBytes          int64
	KeepFilesPerCamera    int
	QueueSizePerCamera    int
	InactiveCloseDuration time.Duration
	FFmpeg                encoder.Config
}

type Manager struct {
	cfg     Config
	store   storage.Store
	metrics *metrics.Metrics
	logger  *log.Logger

	mu      sync.Mutex
	cameras map[uint32]*CameraRecorder
}

func NewManager(cfg Config, store storage.Store, m *metrics.Metrics, logger *log.Logger) *Manager {
	return &Manager{
		cfg:     cfg,
		store:   store,
		metrics: m,
		logger:  logger,
		cameras: make(map[uint32]*CameraRecorder),
	}
}

func (m *Manager) Submit(ctx context.Context, f streamproto.Frame) {
	camera := m.camera(ctx, f.CameraID)
	camera.Submit(f)
}

func (m *Manager) Close() {
	m.mu.Lock()
	cameras := make([]*CameraRecorder, 0, len(m.cameras))
	for _, c := range m.cameras {
		cameras = append(cameras, c)
	}
	m.mu.Unlock()
	for _, c := range cameras {
		c.Close()
	}
}

func (m *Manager) ActiveCameraIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.cameras))
	for id := range m.cameras {
		ids = append(ids, fmt.Sprintf("%d", id))
	}
	sort.Strings(ids)
	return ids
}

func (m *Manager) camera(ctx context.Context, id uint32) *CameraRecorder {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.cameras[id]; c != nil {
		return c
	}
	c := NewCameraRecorder(ctx, id, m.cfg, m.store, m.metrics, m.logger)
	m.cameras[id] = c
	return c
}
