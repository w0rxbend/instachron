// Package httpapi provides the HTTP handler for camera-web-api.
package httpapi

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/w0rxbend/instachron/pkg/mjpeg"
	"github.com/w0rxbend/instachron/services/camera-web-api/internal/camera"
	"github.com/w0rxbend/instachron/services/camera-web-api/internal/rotation"
)

// Handler serves the camera HTTP API.
type Handler struct {
	manager   *camera.Manager
	rotCfg    *rotation.Config
	indexHTML []byte
	logger    *log.Logger
}

// New returns a Handler wired to manager and rotCfg.
func New(manager *camera.Manager, rotCfg *rotation.Config, indexHTML []byte, logger *log.Logger) *Handler {
	return &Handler{manager: manager, rotCfg: rotCfg, indexHTML: indexHTML, logger: logger}
}

// Routes returns the HTTP mux for the camera API.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.handleIndex)
	mux.HandleFunc("GET /cameras", h.handleCameras)
	mux.HandleFunc("GET /cameras/{id}/snapshot", h.handleSnapshot)
	mux.HandleFunc("GET /cameras/{id}/stream", h.handleStream)
	return mux
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(h.indexHTML)
}

// handleCameras returns a JSON array of CameraInfo for every camera seen since
// startup, including cameras that are currently offline.
func (h *Handler) handleCameras(w http.ResponseWriter, r *http.Request) {
	cams := h.manager.KnownCameras(h.rotCfg.Get)
	h.logger.Printf("GET /cameras -> %d camera(s)", len(cams))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cams)
}

// handleSnapshot returns the latest JPEG held in memory for a camera.
func (h *Handler) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	hub := h.manager.HubLookup(id)
	if hub == nil {
		http.NotFound(w, r)
		return
	}

	f := hub.LatestFrame()
	if f == nil {
		http.Error(w, "no frame received yet", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Write(f)
}

// handleStream delivers an MJPEG stream for a camera.
func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	id := r.PathValue("id")
	hub := h.manager.Hub(id)
	h.logger.Printf("GET /cameras/%s/stream -> subscriber attached", id)
	defer h.logger.Printf("GET /cameras/%s/stream -> subscriber detached", id)

	w.Header().Set("Content-Type", mjpeg.ContentType)
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case f, ok := <-ch:
			if !ok {
				return
			}
			if err := mjpeg.WriteFrame(w, f); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
