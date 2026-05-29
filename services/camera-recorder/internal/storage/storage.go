package storage

import (
	"context"
	"io"
	"time"
)

type SegmentInfo struct {
	CameraID        string    `json:"camera_id"`
	FileName        string    `json:"file_name"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	RawDurationSec  float64   `json:"raw_duration_sec"`
	TimelapseFactor int       `json:"timelapse_factor"`
	OutputFPS       int       `json:"output_fps"`
	SizeBytes       int64     `json:"size_bytes"`
	RelativePath    string    `json:"relative_path"`
	DownloadURL     string    `json:"download_url,omitempty"`
}

type SegmentWriter interface {
	io.WriteCloser
	BytesWritten() int64
}

type PendingSegment struct {
	Info   SegmentInfo
	Writer SegmentWriter
}

type ListFilter struct {
	CameraID string
	From     time.Time
	To       time.Time
	Limit    int
}

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type Store interface {
	BeginSegment(ctx context.Context, cameraID string, start time.Time, outputFPS, timelapseFactor int) (*PendingSegment, error)
	CompleteSegment(ctx context.Context, segment *PendingSegment, end time.Time) (SegmentInfo, error)
	DiscardSegment(ctx context.Context, segment *PendingSegment) error
	Prune(ctx context.Context, cameraID string, keep int) error
	List(ctx context.Context, filter ListFilter) ([]SegmentInfo, error)
	Open(ctx context.Context, cameraID, fileName string) (ReadSeekCloser, SegmentInfo, error)
	UsageBytes(ctx context.Context) (int64, error)
	Cameras(ctx context.Context) ([]string, error)
}
