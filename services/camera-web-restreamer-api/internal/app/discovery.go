package app

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

const discoveryInterval = 5 * time.Second

// discovery polls the origin's /cameras endpoint to learn about cameras and
// start per-camera upstream readers. It is the sole goroutine that modifies
// hubManager.started, so no additional locking is needed for that field.
type discovery struct {
	originURL string
	manager   *hubManager
	logger    *log.Logger
	client    *http.Client
	transport *http.Transport
}

func newDiscovery(originURL string, manager *hubManager, logger *log.Logger) *discovery {
	// Shared transport: upstream readers use the same pool but without a timeout.
	transport := &http.Transport{
		DisableCompression:  true,
		MaxIdleConnsPerHost: 64,
		MaxConnsPerHost:     128,
	}
	return &discovery{
		originURL: originURL,
		manager:   manager,
		logger:    logger,
		transport: transport,
		client:    &http.Client{Transport: transport, Timeout: 5 * time.Second},
	}
}

func (d *discovery) run(ctx context.Context) {
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

func (d *discovery) poll(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.originURL+"/cameras", nil)
	if err != nil {
		return
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.logger.Printf("discovery: poll failed: %v", err)
		d.manager.markAllOffline()
		return
	}
	defer resp.Body.Close()

	var cameras []CameraInfo
	if err := json.NewDecoder(resp.Body).Decode(&cameras); err != nil {
		d.logger.Printf("discovery: decode failed: %v", err)
		return
	}

	for _, cam := range cameras {
		hub, isNew := d.manager.ensureCamera(cam.ID, cam.Index, cam.Rotation)
		if isNew || !d.manager.isUpstreamStarted(cam.ID) {
			d.manager.markUpstreamStarted(cam.ID)
			ur := newUpstreamReader(d.originURL, cam.ID, hub, d.logger, d.transport)
			go ur.run(ctx)
		}
	}
}
