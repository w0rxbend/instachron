package restream

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/w0rxbend/instachron/shared/cameras"
)

const discoveryInterval = 5 * time.Second

// Discovery polls the origin's /cameras endpoint to learn about cameras and
// start per-camera upstream readers. It is the sole goroutine that modifies
// Manager.started, so no additional locking is needed for that field.
type Discovery struct {
	originURL string
	manager   *Manager
	proc      Processor
	logger    *log.Logger
	client    *http.Client
	transport *http.Transport
}

// NewDiscovery creates a Discovery that uses proc to transform frames before
// pushing them to hubs. Pass restream.Noop{} for pass-through behaviour.
func NewDiscovery(originURL string, manager *Manager, proc Processor, logger *log.Logger) *Discovery {
	transport := &http.Transport{
		DisableCompression:  true,
		MaxIdleConnsPerHost: 64,
		MaxConnsPerHost:     128,
	}
	return &Discovery{
		originURL: originURL,
		manager:   manager,
		proc:      proc,
		logger:    logger,
		transport: transport,
		client:    &http.Client{Transport: transport, Timeout: 5 * time.Second},
	}
}

// Run polls the origin on a fixed interval until ctx is cancelled.
func (d *Discovery) Run(ctx context.Context) {
	ticker := time.NewTicker(discoveryInterval)
	defer ticker.Stop()

	d.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.poll(ctx)
		}
	}
}

func (d *Discovery) poll(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.originURL+"/cameras", nil)
	if err != nil {
		return
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.logger.Printf("discovery: poll failed: %v", err)
		d.manager.MarkAllOffline()
		return
	}
	defer resp.Body.Close()

	var cams []cameras.CameraInfo
	if err := json.NewDecoder(resp.Body).Decode(&cams); err != nil {
		d.logger.Printf("discovery: decode failed: %v", err)
		return
	}

	for _, cam := range cams {
		hub, isNew := d.manager.EnsureCamera(cam.ID, cam.Index, cam.Rotation)
		if isNew || !d.manager.IsUpstreamStarted(cam.ID) {
			d.manager.MarkUpstreamStarted(cam.ID)
			ur := newUpstreamReader(d.originURL, cam.ID, hub, d.proc, d.logger, d.transport)
			go ur.run(ctx)
		}
	}
}
