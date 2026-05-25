package restream

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"time"
)

const (
	initialBackoff = 500 * time.Millisecond
	maxBackoff     = 10 * time.Second
)

// upstreamReader connects to the origin's MJPEG stream for one camera, parses
// frames, passes them through the Processor, and pushes results to the Hub.
// It self-retries with exponential backoff and marks the camera offline on disconnect.
type upstreamReader struct {
	originURL string
	cameraID  string
	hub       *Hub
	proc      Processor
	logger    *log.Logger
	client    *http.Client
}

func newUpstreamReader(originURL, cameraID string, hub *Hub, proc Processor, logger *log.Logger, transport *http.Transport) *upstreamReader {
	return &upstreamReader{
		originURL: originURL,
		cameraID:  cameraID,
		hub:       hub,
		proc:      proc,
		logger:    logger,
		client:    &http.Client{Transport: transport},
	}
}

func (u *upstreamReader) run(ctx context.Context) {
	backoff := initialBackoff
	for {
		if ctx.Err() != nil {
			return
		}

		frames, err := u.connect(ctx)

		if ctx.Err() != nil {
			return
		}

		u.hub.MarkOffline()

		if frames > 0 {
			backoff = initialBackoff
		} else {
			backoff = min(backoff*2, maxBackoff)
		}

		if err != nil {
			u.logger.Printf("upstream[%s]: %v, retry in %s", u.cameraID, err, backoff)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (u *upstreamReader) connect(ctx context.Context) (frames int, err error) {
	url := u.originURL + "/cameras/" + u.cameraID + "/stream"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return 0, fmt.Errorf("origin returned %s", resp.Status)
	}

	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return 0, fmt.Errorf("parse content-type: %w", err)
	}
	boundary, ok := params["boundary"]
	if !ok {
		return 0, fmt.Errorf("no boundary in content-type")
	}

	hub := u.hub
	mr := multipart.NewReader(resp.Body, boundary)
	for {
		part, err := mr.NextPart()
		if err != nil {
			if ctx.Err() != nil {
				return frames, nil
			}
			return frames, err
		}
		data, err := io.ReadAll(part)
		part.Close()
		if err != nil {
			if ctx.Err() != nil {
				return frames, nil
			}
			return frames, err
		}
		if len(data) > 0 {
			u.proc.Process(data, hub.Push)
			frames++
		}
	}
}
