package app

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

const mjpegBoundary = "instachron"

type apiServer struct {
	manager *hubManager
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
	cameras := s.manager.knownCameras()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cameras)
}

func (s *apiServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h := s.manager.hubLookup(id)
	if h == nil {
		http.NotFound(w, r)
		return
	}
	f := h.latestFrame()
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
	h := s.manager.hubLookup(id)
	if h == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "multipart/x-mixed-replace;boundary="+mjpegBoundary)
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.subscribe()
	defer h.unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case f, ok := <-ch:
			if !ok {
				return
			}
			if err := writeMJPEGFrame(w, f); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeMJPEGFrame(w http.ResponseWriter, f frame) error {
	if _, err := fmt.Fprintf(w, "--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n",
		mjpegBoundary, len(f)); err != nil {
		return err
	}
	if _, err := w.Write(f); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\r\n")
	return err
}
