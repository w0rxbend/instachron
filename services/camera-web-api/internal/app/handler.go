package app

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

func (s *apiServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(s.indexHTML)
}

// handleCameras returns a JSON array of CameraInfo objects for every camera
// seen since startup, including cameras that are currently offline.
func (s *apiServer) handleCameras(w http.ResponseWriter, r *http.Request) {
	cameras := s.manager.knownCameras()
	s.logger.Printf("GET /cameras -> %d camera(s)", len(cameras))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cameras)
}

// handleSnapshot returns the latest JPEG frame held in memory for a camera.
// Returns the last known frame even when the camera is currently offline.
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
	w.Header().Set("X-Accel-Buffering", "no")

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
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body { background: #0a0a0a; color: #ccc; font-family: ui-monospace, monospace; }

    header {
      display: flex; align-items: center; gap: 1rem; padding: .65rem 1.1rem;
      border-bottom: 1px solid #1e1e1e;
    }
    header strong { color: #fff; font-size: .9rem; letter-spacing: .06em; }
    #status { color: #555; font-size: .8rem; }

    #grid { display: flex; flex-wrap: wrap; gap: 4px; padding: 4px; }

    .cam {
      position: relative; flex: 1 1 480px; background: #111;
      min-height: 120px; overflow: hidden;
    }
    .cam img { display: block; width: 100%; height: auto; }

    /* offline overlay */
    .cam.offline img { filter: grayscale(1) brightness(.45); }
    .cam.offline::after {
      content: "OFFLINE";
      position: absolute; inset: 0;
      display: flex; align-items: center; justify-content: center;
      font-size: 1.1rem; letter-spacing: .2em; color: #ff4444;
      background: rgba(0,0,0,.35);
      pointer-events: none;
    }

    .cam-label {
      position: absolute; bottom: 6px; left: 8px;
      background: rgba(0,0,0,.72); padding: 2px 8px;
      font-size: .7rem; border-radius: 2px; letter-spacing: .04em;
    }
    .cam-label .dot {
      display: inline-block; width: 7px; height: 7px;
      border-radius: 50%; margin-right: 5px; vertical-align: middle;
      background: #33cc66;
    }
    .cam.offline .cam-label .dot { background: #ff4444; }

    .cam-actions {
      position: absolute; top: 6px; right: 8px; display: flex; gap: 4px;
    }
    .cam-actions a {
      background: rgba(0,0,0,.72); padding: 2px 8px; font-size: .7rem;
      border-radius: 2px; color: #aaa; text-decoration: none;
    }
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
    const grid   = document.getElementById('grid');
    const status = document.getElementById('status');
    const known  = new Map(); // id -> div element

    function addCamera(cam) {
      const div = document.createElement('div');
      div.className = 'cam' + (cam.online ? '' : ' offline');
      div.dataset.id = cam.id;

      div.innerHTML =
        '<img src="/cameras/' + cam.id + '/stream" alt="camera ' + cam.id + '">' +
        '<span class="cam-label"><span class="dot"></span>cam ' + cam.index + ' &middot; ' + cam.id + '</span>' +
        '<span class="cam-actions">' +
          '<a href="/cameras/' + cam.id + '/snapshot" target="_blank">snapshot</a>' +
          '<a href="/cameras/' + cam.id + '/stream" target="_blank">stream</a>' +
        '</span>';

      grid.appendChild(div);
      known.set(cam.id, div);
    }

    function updateCamera(cam, div) {
      if (cam.online) {
        div.classList.remove('offline');
      } else {
        div.classList.add('offline');
      }
    }

    function refresh() {
      fetch('/cameras')
        .then(r => r.json())
        .then(cameras => {
          const onlineCount = cameras.filter(c => c.online).length;
          status.textContent =
            cameras.length === 0
              ? 'no cameras found'
              : onlineCount + ' online, ' + (cameras.length - onlineCount) + ' offline';

          cameras.forEach(cam => {
            const existing = known.get(cam.id);
            if (existing) {
              updateCamera(cam, existing);
            } else {
              addCamera(cam);
            }
          });
        })
        .catch(() => { status.textContent = 'connection error'; });
    }

    refresh();
    setInterval(refresh, 3000);
  </script>
</body>
</html>`
