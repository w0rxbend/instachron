package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const mjpegBoundary = "instachron"

type apiServer struct {
	manager   *hubManager
	indexHTML []byte
	logger    *log.Logger
}

func (s *apiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /cameras", s.handleCameras)
	mux.HandleFunc("GET /cameras/{id}/snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /cameras/{id}/stream", s.handleStream)
	return mux
}

// handleIndex serves the embedded HTML viewer page.
func (s *apiServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(s.indexHTML)
}

// handleCameras returns a JSON array of discovered camera IDs.
func (s *apiServer) handleCameras(w http.ResponseWriter, r *http.Request) {
	ids := s.manager.knownCameras()
	if ids == nil {
		ids = []string{}
	}
	s.logger.Printf("GET /cameras -> %d camera(s)", len(ids))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ids)
}

// handleSnapshot returns the latest JPEG frame for a camera as a single image.
func (s *apiServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := filepath.Join(s.manager.frameDir, id, "current-image.jpeg")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to read frame", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Write(data)
}

// handleStream delivers an MJPEG (multipart/x-mixed-replace) stream for a
// camera. Browsers render this live via a plain <img> tag.
func (s *apiServer) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	id := r.PathValue("id")
	hub := s.manager.hub(id)
	s.logger.Printf("GET /cameras/%s/stream -> subscriber attached", id)
	defer s.logger.Printf("GET /cameras/%s/stream -> subscriber detached", id)

	w.Header().Set("Content-Type", "multipart/x-mixed-replace;boundary="+mjpegBoundary)
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no") // prevent nginx buffering

	ch := hub.subscribe()
	defer hub.unsubscribe(ch)

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

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>instachron</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { background: #0f0f0f; color: #ccc; font-family: monospace; }
    header { display: flex; align-items: center; gap: 1rem; padding: .6rem 1rem;
             border-bottom: 1px solid #222; font-size: .85rem; }
    header strong { color: #fff; }
    #status { color: #555; }
    #grid { display: flex; flex-wrap: wrap; gap: 3px; padding: 3px; }
    .cam { position: relative; flex: 1 1 480px; background: #1a1a1a; min-height: 120px; }
    .cam img { display: block; width: 100%; height: auto; }
    .cam-label { position: absolute; bottom: 6px; left: 8px;
                 background: rgba(0,0,0,.7); padding: 2px 8px;
                 font-size: .7rem; border-radius: 2px; letter-spacing: .04em; }
    .cam-actions { position: absolute; top: 6px; right: 8px; display: flex; gap: 4px; }
    .cam-actions a { background: rgba(0,0,0,.7); padding: 2px 8px; font-size: .7rem;
                     border-radius: 2px; color: #aaa; text-decoration: none; }
    .cam-actions a:hover { color: #fff; }
  </style>
</head>
<body>
  <header>
    <strong>instachron</strong>
    <span id="status">scanning&hellip;</span>
  </header>
  <div id="grid"></div>
  <script>
    const grid = document.getElementById('grid');
    const status = document.getElementById('status');
    const known = new Set();

    function addCamera(camera) {
      const id = camera.id;
      const div = document.createElement('div');
      div.className = 'cam';
      div.dataset.id = id;
      div.innerHTML =
        '<img src="/cameras/' + id + '/stream" alt="camera ' + id + '">' +
        '<span class="cam-label">camera ' + camera.index + ' · ' + id + '</span>' +
        '<span class="cam-actions">' +
          '<a href="/cameras/' + id + '/snapshot" target="_blank">snapshot</a>' +
          '<a href="/cameras/' + id + '/stream" target="_blank">raw stream</a>' +
        '</span>';
      grid.appendChild(div);
    }

    function refresh() {
      fetch('/cameras')
        .then(r => r.json())
        .then(cameras => {
          status.textContent = cameras.length === 0 ? 'no cameras found' : cameras.length + ' camera(s)';
          cameras.forEach(camera => {
            if (known.has(camera.id)) return;
            known.add(camera.id);
            addCamera(camera);
          });
        })
        .catch(() => { status.textContent = 'error fetching camera list'; });
    }

    refresh();
    setInterval(refresh, 5000);
  </script>
</body>
</html>`
