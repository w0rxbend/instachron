// Package camera manages per-camera frame hubs for the IPC-driven camera-web-api.
// Frames arrive from a single IPC reader goroutine; the hub fans them out to
// HTTP stream subscribers.
package camera

import (
	"sort"
	"sync"
	"time"

	"github.com/w0rxbend/instachron/pkg/cameras"
)

const offlineThreshold = 5 * time.Second

// Hub fans received frames out to every HTTP stream subscriber.
type Hub struct {
	id string

	mu       sync.Mutex
	subs     map[chan []byte]struct{}
	latest   []byte
	online   bool
	lastSeen time.Time
}

func newHub(id string) *Hub {
	return &Hub{
		id:   id,
		subs: make(map[chan []byte]struct{}),
	}
}

// Subscribe registers a new subscriber. The latest known frame is sent
// immediately so the client renders without waiting for the next push.
// Caller must call Unsubscribe when done.
func (h *Hub) Subscribe() chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan []byte, 1)
	h.subs[ch] = struct{}{}

	if h.latest != nil {
		select {
		case ch <- h.latest:
		default:
		}
	}
	return ch
}

// Unsubscribe removes the subscriber and closes its channel.
func (h *Hub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, ch)
	close(ch)
}

// Push delivers a new frame and fans it out to all subscribers.
func (h *Hub) Push(jpeg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.latest = jpeg
	h.online = true
	h.lastSeen = time.Now()

	for ch := range h.subs {
		select {
		case ch <- jpeg:
		default:
		}
	}
}

// MarkOffline records that the camera is no longer sending frames.
func (h *Hub) MarkOffline() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.online = false
}

// IsStale returns true if the camera has been online but silent beyond offlineThreshold.
func (h *Hub) IsStale() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.online && !h.lastSeen.IsZero() && time.Since(h.lastSeen) > offlineThreshold
}

// LatestFrame returns the most recently received JPEG, or nil if none yet.
func (h *Hub) LatestFrame() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.latest
}

// info returns a snapshot of the camera's API state.
func (h *Hub) info(index, rotation int) cameras.CameraInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	return cameras.CameraInfo{ID: h.id, Index: index, Online: h.online, Rotation: rotation}
}

// ---

// Manager owns all per-camera Hubs. Cameras are registered lazily when frames
// arrive and are never removed, so offline cameras remain discoverable.
type Manager struct {
	mu      sync.Mutex
	hubs    map[string]*Hub
	ordered []string
}

// NewManager returns an empty Manager.
func NewManager() *Manager {
	return &Manager{hubs: make(map[string]*Hub)}
}

// CheckLiveness marks stale cameras offline. Call from a periodic ticker.
func (m *Manager) CheckLiveness() {
	m.mu.Lock()
	hubs := make([]*Hub, 0, len(m.hubs))
	for _, h := range m.hubs {
		hubs = append(hubs, h)
	}
	m.mu.Unlock()

	for _, h := range hubs {
		if h.IsStale() {
			h.MarkOffline()
		}
	}
}

// Push delivers a frame to the hub for id, creating it lazily.
func (m *Manager) Push(id string, jpeg []byte) {
	m.getOrCreate(id).Push(jpeg)
}

// MarkOffline marks a specific camera as offline.
func (m *Manager) MarkOffline(id string) {
	m.mu.Lock()
	h, ok := m.hubs[id]
	m.mu.Unlock()
	if ok {
		h.MarkOffline()
	}
}

// MarkAllOffline marks every known camera as offline.
func (m *Manager) MarkAllOffline() {
	m.mu.Lock()
	hubs := make([]*Hub, 0, len(m.hubs))
	for _, h := range m.hubs {
		hubs = append(hubs, h)
	}
	m.mu.Unlock()
	for _, h := range hubs {
		h.MarkOffline()
	}
}

// Hub returns the hub for id, creating it lazily. Used by stream handlers where
// a client may connect before the camera has sent its first frame.
func (m *Manager) Hub(id string) *Hub {
	return m.getOrCreate(id)
}

// HubLookup returns the hub for id, or nil if the camera has never been seen.
func (m *Manager) HubLookup(id string) *Hub {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hubs[id]
}

// KnownCameras returns camera info for every camera seen since startup.
// rotFn returns the configured rotation angle for a given camera ID.
func (m *Manager) KnownCameras(rotFn func(id string) int) []cameras.CameraInfo {
	m.mu.Lock()
	ids := make([]string, len(m.ordered))
	copy(ids, m.ordered)
	m.mu.Unlock()

	sort.Strings(ids)

	infos := make([]cameras.CameraInfo, 0, len(ids))
	for i, id := range ids {
		m.mu.Lock()
		h := m.hubs[id]
		m.mu.Unlock()
		infos = append(infos, h.info(i, rotFn(id)))
	}
	return infos
}

func (m *Manager) getOrCreate(id string) *Hub {
	m.mu.Lock()
	defer m.mu.Unlock()

	if h, ok := m.hubs[id]; ok {
		return h
	}
	h := newHub(id)
	m.hubs[id] = h
	m.ordered = append(m.ordered, id)
	return h
}
