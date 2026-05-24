package main

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
	upstreamInitialBackoff = 500 * time.Millisecond
	upstreamMaxBackoff     = 10 * time.Second
)

// upstreamReader connects to the origin's MJPEG stream for one camera, parses
// frames, and pushes them to the local hub. It self-retries with exponential
// backoff and marks the camera offline on every disconnect.
type upstreamReader struct {
	originURL string
	cameraID  string
	hub       *cameraHub
	logger    *log.Logger
	client    *http.Client
}

func newUpstreamReader(originURL, cameraID string, hub *cameraHub, logger *log.Logger, transport *http.Transport) *upstreamReader {
	return &upstreamReader{
		originURL: originURL,
		cameraID:  cameraID,
		hub:       hub,
		logger:    logger,
		// No timeout on the client: MJPEG streams are long-lived.
		client: &http.Client{Transport: transport},
	}
}

func (u *upstreamReader) run(ctx context.Context) {
	backoff := upstreamInitialBackoff
	for {
		if ctx.Err() != nil {
			return
		}

		frames, err := u.connect(ctx)

		if ctx.Err() != nil {
			return
		}

		u.hub.markOffline()

		if frames > 0 {
			// Had a live session — reset backoff so the next reconnect is fast.
			backoff = upstreamInitialBackoff
		} else {
			backoff = min(backoff*2, upstreamMaxBackoff)
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

// connect dials the origin MJPEG stream, reads parts until EOF or error, and
// returns the count of frames pushed and the first error encountered.
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
			u.hub.push(data)
			frames++
		}
	}
}
