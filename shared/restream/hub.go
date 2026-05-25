package restream

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/w0rxbend/instachron/shared/cameras"
)

const offlineThreshold = 5 * time.Second

// Hub fans received frames out to all active HTTP stream subscribers.
// The latest frame and liveness state are stored via atomics for lock-free reads
// on the hot path; subscriber management uses a mutex.
type Hub struct {
	id string

	latest   atomic.Pointer[[]byte]
	lastSeen atomic.Int64
	online   atomic.Bool

	// infoMu protects index and rotation, updated from discovery polls.
	infoMu   sync.RWMutex
	index    int
	rotation int

	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func newHub(id string, index, rotation int) *Hub {
	return &Hub{
		id:       id,
		index:    index,
		rotation: rotation,
		subs:     make(map[chan []byte]struct{}),
	}
}

// Subscribe registers a new subscriber channel. The latest known frame is sent
// immediately so the client renders without waiting for the next push.
// Caller must call Unsubscribe when done.
func (h *Hub) Subscribe() chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan []byte, 1)
	h.subs[ch] = struct{}{}

	if p := h.latest.Load(); p != nil {
		select {
		case ch <- *p:
		default:
		}
	}
	return ch
}

// Unsubscribe removes the subscriber and closes its channel.
func (h *Hub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

// Push stores jpeg as the latest frame and fans it out to all subscribers.
// Slow subscribers get the frame dropped rather than blocking the push path.
func (h *Hub) Push(jpeg []byte) {
	h.latest.Store(&jpeg)
	h.lastSeen.Store(time.Now().UnixNano())
	h.online.Store(true)

	h.mu.Lock()
	for ch := range h.subs {
		select {
		case ch <- jpeg:
		default:
		}
	}
	h.mu.Unlock()
}

// MarkOffline records that the camera is no longer sending frames.
func (h *Hub) MarkOffline() {
	h.online.Store(false)
}

// IsStale returns true if the camera was online but has been silent beyond offlineThreshold.
func (h *Hub) IsStale() bool {
	if !h.online.Load() {
		return false
	}
	ls := h.lastSeen.Load()
	return ls != 0 && time.Since(time.Unix(0, ls)) > offlineThreshold
}

// UpdateInfo refreshes index and rotation from the latest discovery poll.
func (h *Hub) UpdateInfo(index, rotation int) {
	h.infoMu.Lock()
	h.index = index
	h.rotation = rotation
	h.infoMu.Unlock()
}

// Info returns the camera's current API state.
func (h *Hub) Info() cameras.CameraInfo {
	h.infoMu.RLock()
	idx, rot := h.index, h.rotation
	h.infoMu.RUnlock()
	return cameras.CameraInfo{ID: h.id, Index: idx, Online: h.online.Load(), Rotation: rot}
}

// LatestFrame returns the most recently received JPEG, or nil if none yet.
func (h *Hub) LatestFrame() []byte {
	if p := h.latest.Load(); p != nil {
		return *p
	}
	return nil
}

// ---

// Manager owns all per-camera Hubs. Cameras are added lazily on discovery and
// never removed, so offline cameras remain discoverable via the API.
type Manager struct {
	mu      sync.RWMutex
	hubs    map[string]*Hub
	ordered []string
	started map[string]bool // cameras with an upstream goroutine already running
}

// NewManager returns an empty Manager.
func NewManager() *Manager {
	return &Manager{
		hubs:    make(map[string]*Hub),
		started: make(map[string]bool),
	}
}

// EnsureCamera creates the hub on first discovery; on subsequent calls it only
// updates the cached index/rotation. Returns (hub, isNew).
func (m *Manager) EnsureCamera(id string, index, rotation int) (*Hub, bool) {
	m.mu.Lock()
	if h, ok := m.hubs[id]; ok {
		m.mu.Unlock()
		h.UpdateInfo(index, rotation)
		return h, false
	}
	h := newHub(id, index, rotation)
	m.hubs[id] = h
	m.ordered = append(m.ordered, id)
	m.mu.Unlock()
	return h, true
}

// MarkUpstreamStarted records that an upstream goroutine is running for id.
func (m *Manager) MarkUpstreamStarted(id string) {
	m.mu.Lock()
	m.started[id] = true
	m.mu.Unlock()
}

// IsUpstreamStarted reports whether an upstream goroutine is already running.
func (m *Manager) IsUpstreamStarted(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started[id]
}

// HubLookup returns the Hub for id, or nil if it has never been seen.
func (m *Manager) HubLookup(id string) *Hub {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hubs[id]
}

// MarkAllOffline marks every known camera as offline.
func (m *Manager) MarkAllOffline() {
	m.mu.RLock()
	hubs := make([]*Hub, 0, len(m.hubs))
	for _, h := range m.hubs {
		hubs = append(hubs, h)
	}
	m.mu.RUnlock()
	for _, h := range hubs {
		h.MarkOffline()
	}
}

// CheckLiveness marks cameras offline when they exceed the silence threshold.
// Designed to be called from a periodic ticker goroutine.
func (m *Manager) CheckLiveness() {
	m.mu.RLock()
	hubs := make([]*Hub, 0, len(m.hubs))
	for _, h := range m.hubs {
		hubs = append(hubs, h)
	}
	m.mu.RUnlock()
	for _, h := range hubs {
		if h.IsStale() {
			h.MarkOffline()
		}
	}
}

// KnownCameras returns camera info for every camera seen since startup,
// sorted by ID for stable ordering.
func (m *Manager) KnownCameras() []cameras.CameraInfo {
	m.mu.RLock()
	ids := make([]string, len(m.ordered))
	copy(ids, m.ordered)
	m.mu.RUnlock()

	sort.Strings(ids)

	infos := make([]cameras.CameraInfo, 0, len(ids))
	for _, id := range ids {
		m.mu.RLock()
		h := m.hubs[id]
		m.mu.RUnlock()
		infos = append(infos, h.Info())
	}
	return infos
}
