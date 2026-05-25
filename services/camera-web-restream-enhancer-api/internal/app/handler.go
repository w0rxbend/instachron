package app

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/w0rxbend/instachron/pkg/mjpeg"
	"github.com/w0rxbend/instachron/pkg/restream"
)

type apiServer struct {
	manager *restream.Manager
	logger  *log.Logger
}

func (s *apiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /cameras", s.handleCameras)
	mux.HandleFunc("GET /cameras/{id}/snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /cameras/{id}/stream", s.handleStream)
	return mux
}

func (s *apiServer) handleCameras(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.manager.KnownCameras())
}

func (s *apiServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h := s.manager.HubLookup(id)
	if h == nil {
		http.NotFound(w, r)
		return
	}

	f := h.LatestFrame()
	if f == nil {
		http.Error(w, "no frame received yet", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Write(f)
}

func (s *apiServer) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	id := r.PathValue("id")
	h := s.manager.HubLookup(id)
	if h == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", mjpeg.ContentType)
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.Subscribe()
	defer h.Unsubscribe(ch)

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
