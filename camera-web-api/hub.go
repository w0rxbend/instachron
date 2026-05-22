package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// frame is a single JPEG snapshot broadcast to all active stream subscribers.
type frame []byte

// cameraHub watches one camera's current-image.jpeg and fans new frames out
// to every subscriber channel. The background goroutine starts on the first
// subscribe call and stops when the last subscriber disconnects.
type cameraHub struct {
	imgPath string
	poll    time.Duration
	parent  context.Context

	mu     sync.Mutex
	subs   map[chan frame]struct{}
	latest frame
	cancel context.CancelFunc
}

func newCameraHub(imgPath string, poll time.Duration, parent context.Context) *cameraHub {
	return &cameraHub{
		imgPath: imgPath,
		poll:    poll,
		parent:  parent,
		subs:    make(map[chan frame]struct{}),
	}
}

// subscribe registers a new subscriber and returns its channel. The latest
// known frame is queued immediately so the client renders without waiting for
// the next poll tick. Caller must call unsubscribe when done.
func (h *cameraHub) subscribe() chan frame {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan frame, 1)
	h.subs[ch] = struct{}{}

	if h.latest == nil {
		if data, err := os.ReadFile(h.imgPath); err == nil {
			h.latest = data
		}
	}

	if h.cancel == nil {
		ctx, cancel := context.WithCancel(h.parent)
		h.cancel = cancel
		go h.run(ctx)
	}

	if h.latest != nil {
		select {
		case ch <- h.latest:
		default:
		}
	}

	return ch
}

// unsubscribe removes the subscriber and closes its channel. Stops the
// background goroutine when no subscribers remain.
func (h *cameraHub) unsubscribe(ch chan frame) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.subs, ch)
	close(ch)

	if len(h.subs) == 0 && h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

func (h *cameraHub) run(ctx context.Context) {
	ticker := time.NewTicker(h.poll)
	defer ticker.Stop()

	var lastMod time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(h.imgPath)
			if err != nil || !info.ModTime().After(lastMod) {
				continue
			}
			data, err := os.ReadFile(h.imgPath)
			if err != nil {
				continue
			}
			lastMod = info.ModTime()
			h.broadcast(data)
		}
	}
}

func (h *cameraHub) broadcast(f frame) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.latest = f
	for ch := range h.subs {
		select {
		case ch <- f:
		default: // drop frame for slow subscribers rather than block
		}
	}
}

// hubManager owns all per-camera hubs and runs a background camera discovery
// loop that keeps the known camera list up to date.
type hubManager struct {
	frameDir  string
	poll      time.Duration
	serverCtx context.Context

	mu      sync.Mutex
	hubs    map[string]*cameraHub
	cameras []string
}

func newHubManager(frameDir string, poll time.Duration, serverCtx context.Context) *hubManager {
	return &hubManager{
		frameDir:  frameDir,
		poll:      poll,
		serverCtx: serverCtx,
		hubs:      make(map[string]*cameraHub),
	}
}

// run is the camera discovery loop. It scans frameDir at 4× poll interval and
// updates the known camera list. Blocks until serverCtx is cancelled.
func (m *hubManager) run() {
	ticker := time.NewTicker(m.poll * 4)
	defer ticker.Stop()

	m.discover()
	for {
		select {
		case <-m.serverCtx.Done():
			return
		case <-ticker.C:
			m.discover()
		}
	}
}

func (m *hubManager) discover() {
	ids, _ := discoverCameras(m.frameDir)
	if ids == nil {
		ids = []string{}
	}
	m.mu.Lock()
	m.cameras = ids
	m.mu.Unlock()
}

// knownCameras returns a snapshot of the currently discovered camera IDs.
func (m *hubManager) knownCameras() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.cameras))
	copy(cp, m.cameras)
	return cp
}

// hub returns the hub for the given camera ID, creating it lazily if needed.
func (m *hubManager) hub(id string) *cameraHub {
	imgPath := filepath.Join(m.frameDir, id, "current-image.jpeg")
	m.mu.Lock()
	defer m.mu.Unlock()

	h, ok := m.hubs[id]
	if !ok {
		h = newCameraHub(imgPath, m.poll, m.serverCtx)
		m.hubs[id] = h
	}
	return h
}

func discoverCameras(frameDir string) ([]string, error) {
	entries, err := os.ReadDir(frameDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(frameDir, e.Name(), "current-image.jpeg")); err == nil {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}
