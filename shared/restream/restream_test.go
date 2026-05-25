package restream_test

import (
	"testing"
	"time"

	"github.com/w0rxbend/instachron/shared/restream"
)

// --- Hub tests ---

func TestHubSubscribeReceivesLatestFrame(t *testing.T) {
	h := restream.NewManager()
	hub, _ := h.EnsureCamera("1", 0, 0)

	jpeg := []byte{0xFF, 0xD8, 0xAA, 0xFF, 0xD9}
	hub.Push(jpeg)

	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	select {
	case got := <-ch:
		if string(got) != string(jpeg) {
			t.Fatalf("got %v, want %v", got, jpeg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("subscribe: timeout waiting for initial frame")
	}
}

func TestHubPushFansOut(t *testing.T) {
	mgr := restream.NewManager()
	hub, _ := mgr.EnsureCamera("cam1", 0, 0)

	ch1 := hub.Subscribe()
	ch2 := hub.Subscribe()
	defer hub.Unsubscribe(ch1)
	defer hub.Unsubscribe(ch2)

	// Drain the initial frame sent on subscribe (hub has no prior frame here).
	jpeg := []byte("newframe")
	hub.Push(jpeg)

	for _, ch := range []chan []byte{ch1, ch2} {
		select {
		case got := <-ch:
			if string(got) != string(jpeg) {
				t.Errorf("got %q, want %q", got, jpeg)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("fan-out: timeout waiting for frame")
		}
	}
}

func TestHubUnsubscribeClosesChannel(t *testing.T) {
	mgr := restream.NewManager()
	hub, _ := mgr.EnsureCamera("cam2", 0, 0)

	ch := hub.Subscribe()
	hub.Unsubscribe(ch)

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after Unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel not closed after Unsubscribe")
	}
}

func TestHubMarkOffline(t *testing.T) {
	mgr := restream.NewManager()
	hub, _ := mgr.EnsureCamera("cam3", 0, 0)

	hub.Push([]byte("frame"))
	if !hub.Info().Online {
		t.Fatal("hub should be online after push")
	}

	hub.MarkOffline()
	if hub.Info().Online {
		t.Fatal("hub should be offline after MarkOffline")
	}
}

func TestHubIsStale(t *testing.T) {
	mgr := restream.NewManager()
	hub, _ := mgr.EnsureCamera("cam4", 0, 0)

	// Hub that has never received a frame is not stale.
	if hub.IsStale() {
		t.Fatal("never-pushed hub should not be stale")
	}

	hub.Push([]byte("frame"))
	// Just pushed — not stale yet.
	if hub.IsStale() {
		t.Fatal("hub should not be stale immediately after push")
	}
}

func TestHubLatestFrame(t *testing.T) {
	mgr := restream.NewManager()
	hub, _ := mgr.EnsureCamera("cam5", 0, 0)

	if hub.LatestFrame() != nil {
		t.Fatal("LatestFrame should be nil before any push")
	}

	jpeg := []byte("jpeg_data")
	hub.Push(jpeg)
	if string(hub.LatestFrame()) != string(jpeg) {
		t.Fatalf("LatestFrame = %q, want %q", hub.LatestFrame(), jpeg)
	}
}

// --- Manager tests ---

func TestManagerEnsureCameraIsNew(t *testing.T) {
	mgr := restream.NewManager()
	_, isNew := mgr.EnsureCamera("1", 0, 0)
	if !isNew {
		t.Fatal("first EnsureCamera should return isNew=true")
	}
	_, isNew = mgr.EnsureCamera("1", 0, 0)
	if isNew {
		t.Fatal("second EnsureCamera for same id should return isNew=false")
	}
}

func TestManagerHubLookup(t *testing.T) {
	mgr := restream.NewManager()
	if mgr.HubLookup("missing") != nil {
		t.Fatal("HubLookup for unknown id should return nil")
	}
	mgr.EnsureCamera("known", 0, 0)
	if mgr.HubLookup("known") == nil {
		t.Fatal("HubLookup for known id should return non-nil")
	}
}

func TestManagerMarkAllOffline(t *testing.T) {
	mgr := restream.NewManager()
	h1, _ := mgr.EnsureCamera("a", 0, 0)
	h2, _ := mgr.EnsureCamera("b", 0, 0)
	h1.Push([]byte("f"))
	h2.Push([]byte("f"))

	mgr.MarkAllOffline()

	if h1.Info().Online || h2.Info().Online {
		t.Fatal("all cameras should be offline after MarkAllOffline")
	}
}

func TestManagerKnownCameras(t *testing.T) {
	mgr := restream.NewManager()
	mgr.EnsureCamera("2", 1, 90)
	mgr.EnsureCamera("1", 0, 0)

	cams := mgr.KnownCameras()
	if len(cams) != 2 {
		t.Fatalf("KnownCameras len = %d, want 2", len(cams))
	}
	// Should be sorted by ID string.
	if cams[0].ID != "1" || cams[1].ID != "2" {
		t.Fatalf("KnownCameras order wrong: %v", cams)
	}
}

func TestManagerUpstreamStarted(t *testing.T) {
	mgr := restream.NewManager()
	if mgr.IsUpstreamStarted("x") {
		t.Fatal("IsUpstreamStarted should be false before MarkUpstreamStarted")
	}
	mgr.MarkUpstreamStarted("x")
	if !mgr.IsUpstreamStarted("x") {
		t.Fatal("IsUpstreamStarted should be true after MarkUpstreamStarted")
	}
}

// --- Processor tests ---

func TestNoopProcessor(t *testing.T) {
	var received []byte
	push := func(b []byte) { received = b }

	restream.Noop{}.Process([]byte("data"), push)

	if string(received) != "data" {
		t.Fatalf("Noop.Process: got %q, want %q", received, "data")
	}
}
