package main

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const offlineThreshold = 5 * time.Second

// frame is a single JPEG snapshot broadcast to all active stream subscribers.
// Shared across goroutines read-only — never mutated after push.
type frame []byte

// CameraInfo is the API representation mirroring camera-web-api's response shape.
type CameraInfo struct {
	ID       string `json:"id"`
	Index    int    `json:"index"`
	Online   bool   `json:"online"`
	Rotation int    `json:"rotation"`
}

// cameraHub fans received frames out to every HTTP stream subscriber.
// The latest frame is stored via atomic.Pointer for lock-free snapshot reads.
type cameraHub struct {
	id string

	latestPtr atomic.Pointer[frame]
	lastSeen  atomic.Int64
	online    atomic.Bool

	infoMu   sync.RWMutex
	index    int
	rotation int

	mu   sync.Mutex
	subs map[chan frame]struct{}
}

func newCameraHub(id string, index, rotation int) *cameraHub {
	return &cameraHub{
		id:       id,
		index:    index,
		rotation: rotation,
		subs:     make(map[chan frame]struct{}),
	}
}

func (h *cameraHub) subscribe() chan frame {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan frame, 1)
	h.subs[ch] = struct{}{}

	if p := h.latestPtr.Load(); p != nil {
		select {
		case ch <- *p:
		default:
		}
	}
	return ch
}

func (h *cameraHub) unsubscribe(ch chan frame) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *cameraHub) push(jpeg []byte) {
	f := frame(jpeg)
	h.latestPtr.Store(&f)
	h.lastSeen.Store(time.Now().UnixNano())
	h.online.Store(true)

	h.mu.Lock()
	for ch := range h.subs {
		select {
		case ch <- f:
		default:
		}
	}
	h.mu.Unlock()
}

func (h *cameraHub) markOffline() {
	h.online.Store(false)
}

func (h *cameraHub) isStale() bool {
	if !h.online.Load() {
		return false
	}
	ls := h.lastSeen.Load()
	return ls != 0 && time.Since(time.Unix(0, ls)) > offlineThreshold
}

func (h *cameraHub) updateInfo(index, rotation int) {
	h.infoMu.Lock()
	h.index = index
	h.rotation = rotation
	h.infoMu.Unlock()
}

func (h *cameraHub) info() CameraInfo {
	h.infoMu.RLock()
	idx, rot := h.index, h.rotation
	h.infoMu.RUnlock()
	return CameraInfo{ID: h.id, Index: idx, Online: h.online.Load(), Rotation: rot}
}

func (h *cameraHub) latestFrame() frame {
	if p := h.latestPtr.Load(); p != nil {
		return *p
	}
	return nil
}

// ---

type hubManager struct {
	mu      sync.RWMutex
	hubs    map[string]*cameraHub
	ordered []string
	started map[string]bool
}

func newHubManager() *hubManager {
	return &hubManager{
		hubs:    make(map[string]*cameraHub),
		started: make(map[string]bool),
	}
}

func (m *hubManager) ensureCamera(id string, index, rotation int) (*cameraHub, bool) {
	m.mu.Lock()
	if h, ok := m.hubs[id]; ok {
		m.mu.Unlock()
		h.updateInfo(index, rotation)
		return h, false
	}
	h := newCameraHub(id, index, rotation)
	m.hubs[id] = h
	m.ordered = append(m.ordered, id)
	m.mu.Unlock()
	return h, true
}

func (m *hubManager) markUpstreamStarted(id string) {
	m.mu.Lock()
	m.started[id] = true
	m.mu.Unlock()
}

func (m *hubManager) isUpstreamStarted(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started[id]
}

func (m *hubManager) hubLookup(id string) *cameraHub {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hubs[id]
}

func (m *hubManager) markAllOffline() {
	m.mu.RLock()
	hubs := make([]*cameraHub, 0, len(m.hubs))
	for _, h := range m.hubs {
		hubs = append(hubs, h)
	}
	m.mu.RUnlock()
	for _, h := range hubs {
		h.markOffline()
	}
}

func (m *hubManager) checkLiveness() {
	m.mu.RLock()
	hubs := make([]*cameraHub, 0, len(m.hubs))
	for _, h := range m.hubs {
		hubs = append(hubs, h)
	}
	m.mu.RUnlock()
	for _, h := range hubs {
		if h.isStale() {
			h.markOffline()
		}
	}
}

func (m *hubManager) knownCameras() []CameraInfo {
	m.mu.RLock()
	ids := make([]string, len(m.ordered))
	copy(ids, m.ordered)
	m.mu.RUnlock()

	sort.Strings(ids)

	infos := make([]CameraInfo, 0, len(ids))
	for _, id := range ids {
		m.mu.RLock()
		h := m.hubs[id]
		m.mu.RUnlock()
		infos = append(infos, h.info())
	}
	return infos
}
