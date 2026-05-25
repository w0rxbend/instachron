// Package cameras defines the shared camera DTO used by web API and restream services.
package cameras

// CameraInfo is the JSON representation of a camera's current state.
// All HTTP /cameras endpoints return []CameraInfo with this exact shape.
type CameraInfo struct {
	ID       string `json:"id"`
	Index    int    `json:"index"`
	Online   bool   `json:"online"`
	Rotation int    `json:"rotation"`
}
