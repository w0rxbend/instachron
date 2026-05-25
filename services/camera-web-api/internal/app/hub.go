package app

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

const offlineThreshold = 5 * time.Second

// frame is a single JPEG snapshot broadcast to all active stream subscribers.
type frame []byte

// CameraInfo is the API representation of a camera.
type CameraInfo struct {
	ID       string `json:"id"`
	Index    int    `json:"index"`
	Online   bool   `json:"online"`
	Rotation int    `json:"rotation"`
}

// cameraHub fans received frames out to every HTTP stream subscriber.
// Frames are pushed via push(); the hub never touches the filesystem.
type cameraHub struct {
	id string

	mu       sync.Mutex
	subs     map[chan frame]struct{}
	latest   frame
	online   bool
	lastSeen time.Time
}

func newCameraHub(id string) *cameraHub {
	return &cameraHub{
		id:   id,
		subs: make(map[chan frame]struct{}),
	}
}

// subscribe registers a new subscriber and returns its channel. The latest
// known frame is sent immediately so the client renders without waiting for
// the next push. Caller must call unsubscribe when done.
func (h *cameraHub) subscribe() chan frame {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan frame, 1)
	h.subs[ch] = struct{}{}

	if h.latest != nil {
		select {
		case ch <- h.latest:
		default:
		}
	}
	return ch
}

// unsubscribe removes the subscriber and closes its channel.
func (h *cameraHub) unsubscribe(ch chan frame) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.subs, ch)
	close(ch)
}

// push delivers a new frame and fans it out to all subscribers.
func (h *cameraHub) push(jpeg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.latest = jpeg
	h.online = true
	h.lastSeen = time.Now()

	for ch := range h.subs {
		select {
		case ch <- jpeg:
		default: // drop frame for slow subscribers rather than block
		}
	}
}

// markOffline records that the camera is no longer sending frames.
func (h *cameraHub) markOffline() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.online = false
}

// isStale returns true if the camera has been online but silent for longer
// than the offline threshold. Called by the liveness goroutine.
func (h *cameraHub) isStale() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.online && !h.lastSeen.IsZero() && time.Since(h.lastSeen) > offlineThreshold
}

// info returns a snapshot of the camera's current API state.
func (h *cameraHub) info(index int, rotation int) CameraInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	return CameraInfo{ID: h.id, Index: index, Online: h.online, Rotation: rotation}
}

// latestFrame returns the most recently received JPEG, or nil if none yet.
func (h *cameraHub) latestFrame() frame {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.latest
}

// ---

// hubManager owns all per-camera hubs. Cameras are registered lazily when
// frames arrive via dispatch; they are never removed so offline cameras remain
// discoverable. A background liveness goroutine marks cameras offline when
// they go silent beyond offlineThreshold.
type hubManager struct {
	serverCtx context.Context
	rotation  *rotationConfig

	mu      sync.Mutex
	hubs    map[string]*cameraHub
	ordered []string // IDs in first-seen order, for stable indexing
}

func newHubManager(serverCtx context.Context, rotation *rotationConfig) *hubManager {
	return &hubManager{
		serverCtx: serverCtx,
		rotation:  rotation,
		hubs:      make(map[string]*cameraHub),
	}
}

// run is the liveness check loop. It runs until serverCtx is cancelled.
func (m *hubManager) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.serverCtx.Done():
			return
		case <-ticker.C:
			m.checkLiveness()
		}
	}
}

func (m *hubManager) checkLiveness() {
	m.mu.Lock()
	hubs := make([]*cameraHub, 0, len(m.hubs))
	for _, h := range m.hubs {
		hubs = append(hubs, h)
	}
	m.mu.Unlock()

	for _, h := range hubs {
		if h.isStale() {
			h.markOffline()
		}
	}
}

// dispatch receives an incoming frame for a camera, creating the hub lazily,
// applies any configured rotation, and pushes the frame to all subscribers.
func (m *hubManager) dispatch(cameraID uint32, jpeg []byte) {
	id := fmt.Sprintf("%d", cameraID)
	if deg := m.rotation.get(id); deg != 0 {
		jpeg = applyRotation(jpeg, deg)
	}
	h := m.getOrCreate(id)
	h.push(jpeg)
}

// markOffline marks a specific camera as offline (called on CAMERA_OFFLINE IPC message).
func (m *hubManager) markOffline(cameraID uint32) {
	id := fmt.Sprintf("%d", cameraID)
	m.mu.Lock()
	h, ok := m.hubs[id]
	m.mu.Unlock()
	if ok {
		h.markOffline()
	}
}

// markAllOffline marks every known camera as offline (called when IPC connection drops).
func (m *hubManager) markAllOffline() {
	m.mu.Lock()
	hubs := make([]*cameraHub, 0, len(m.hubs))
	for _, h := range m.hubs {
		hubs = append(hubs, h)
	}
	m.mu.Unlock()

	for _, h := range hubs {
		h.markOffline()
	}
}

// hub returns the hub for the given string camera ID, creating it lazily.
// Used by stream and snapshot handlers that address cameras by string ID.
func (m *hubManager) hub(id string) *cameraHub {
	return m.getOrCreate(id)
}

// hubLookup returns the hub for the given ID, or nil if it has never been seen.
func (m *hubManager) hubLookup(id string) *cameraHub {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hubs[id]
}

func (m *hubManager) getOrCreate(id string) *cameraHub {
	m.mu.Lock()
	defer m.mu.Unlock()

	if h, ok := m.hubs[id]; ok {
		return h
	}

	h := newCameraHub(id)
	m.hubs[id] = h
	m.ordered = append(m.ordered, id)
	return h
}

// knownCameras returns camera info for every camera seen since startup,
// sorted by numeric ID for stable ordering.
func (m *hubManager) knownCameras() []CameraInfo {
	m.mu.Lock()
	ids := make([]string, len(m.ordered))
	copy(ids, m.ordered)
	m.mu.Unlock()

	sort.Strings(ids)

	infos := make([]CameraInfo, 0, len(ids))
	for i, id := range ids {
		m.mu.Lock()
		h := m.hubs[id]
		m.mu.Unlock()
		infos = append(infos, h.info(i, m.rotation.get(id)))
	}
	return infos
}
