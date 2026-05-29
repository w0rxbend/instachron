package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

type Local struct {
	root string
}

func NewLocal(root string) *Local {
	return &Local{root: root}
}

type localPending struct {
	PendingSegment
	tmpPath   string
	finalPath string
}

type countingFile struct {
	*os.File
	n atomic.Int64
}

func (f *countingFile) Write(p []byte) (int, error) {
	n, err := f.File.Write(p)
	f.n.Add(int64(n))
	return n, err
}

func (f *countingFile) BytesWritten() int64 {
	return f.n.Load()
}

func (l *Local) BeginSegment(ctx context.Context, cameraID string, start time.Time, outputFPS, timelapseFactor int) (*PendingSegment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(l.cameraDir(cameraID), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(l.tmpDir(), 0o755); err != nil {
		return nil, err
	}

	fileName := fmt.Sprintf("%s_%d.mp4", start.UTC().Format("20060102T150405Z"), start.UnixNano())
	tmpPath := filepath.Join(l.tmpDir(), fmt.Sprintf("camera-%s-%s.tmp", safeName(cameraID), fileName))
	finalPath := filepath.Join(l.cameraDir(cameraID), fileName)

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	p := &PendingSegment{
		Info: SegmentInfo{
			CameraID:        cameraID,
			FileName:        fileName,
			StartedAt:       start.UTC(),
			TimelapseFactor: timelapseFactor,
			OutputFPS:       outputFPS,
			RelativePath:    filepath.ToSlash(filepath.Join("camera-"+safeName(cameraID), fileName)),
		},
		Writer: &countingFile{File: f},
	}
	return (&localPending{PendingSegment: *p, tmpPath: tmpPath, finalPath: finalPath}).asPending(), nil
}

func (l *Local) CompleteSegment(ctx context.Context, segment *PendingSegment, end time.Time) (SegmentInfo, error) {
	if err := ctx.Err(); err != nil {
		return SegmentInfo{}, err
	}
	lp, err := localFromPending(segment)
	if err != nil {
		return SegmentInfo{}, err
	}
	info := lp.Info
	info.EndedAt = end.UTC()
	info.RawDurationSec = end.Sub(info.StartedAt).Seconds()
	info.SizeBytes = lp.Writer.BytesWritten()

	if err := os.Rename(lp.tmpPath, lp.finalPath); err != nil {
		return SegmentInfo{}, err
	}
	if stat, err := os.Stat(lp.finalPath); err == nil {
		info.SizeBytes = stat.Size()
	}
	if err := l.writeMetadata(info); err != nil {
		return SegmentInfo{}, err
	}
	_ = os.Remove(filepath.Join(l.cameraDir(info.CameraID), "latest.mp4"))
	_ = os.Symlink(info.FileName, filepath.Join(l.cameraDir(info.CameraID), "latest.mp4"))
	return info, nil
}

func (l *Local) DiscardSegment(ctx context.Context, segment *PendingSegment) error {
	if segment == nil {
		return nil
	}
	lp, err := localFromPending(segment)
	if err != nil {
		return err
	}
	_ = segment.Writer.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Remove(lp.tmpPath)
}

func (l *Local) Prune(ctx context.Context, cameraID string, keep int) error {
	if keep <= 0 {
		return nil
	}
	files, err := l.List(ctx, ListFilter{CameraID: cameraID})
	if err != nil {
		return err
	}
	if len(files) <= keep {
		return nil
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].StartedAt.Before(files[j].StartedAt)
	})
	for _, info := range files[:len(files)-keep] {
		if err := ctx.Err(); err != nil {
			return err
		}
		base := filepath.Join(l.cameraDir(cameraID), info.FileName)
		_ = os.Remove(base)
		_ = os.Remove(base + ".json")
	}
	return nil
}

func (l *Local) List(ctx context.Context, filter ListFilter) ([]SegmentInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cameraIDs := []string{filter.CameraID}
	if filter.CameraID == "" {
		var err error
		cameraIDs, err = l.Cameras(ctx)
		if err != nil {
			return nil, err
		}
	}

	var out []SegmentInfo
	for _, id := range cameraIDs {
		entries, err := os.ReadDir(l.cameraDir(id))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".mp4") || e.Name() == "latest.mp4" {
				continue
			}
			info, err := l.readMetadata(id, e.Name())
			if err != nil {
				continue
			}
			if !filter.From.IsZero() && info.StartedAt.Before(filter.From) {
				continue
			}
			if !filter.To.IsZero() && info.StartedAt.After(filter.To) {
				continue
			}
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func (l *Local) Open(ctx context.Context, cameraID, fileName string) (ReadSeekCloser, SegmentInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, SegmentInfo{}, err
	}
	if !validFileName(fileName) {
		return nil, SegmentInfo{}, fs.ErrInvalid
	}
	info, err := l.readMetadata(cameraID, fileName)
	if err != nil {
		return nil, SegmentInfo{}, err
	}
	f, err := os.Open(filepath.Join(l.cameraDir(cameraID), fileName))
	if err != nil {
		return nil, SegmentInfo{}, err
	}
	return f, info, nil
}

func (l *Local) UsageBytes(ctx context.Context) (int64, error) {
	var total int64
	err := filepath.WalkDir(l.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".mp4") || d.Name() == "latest.mp4" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	return total, err
}

func (l *Local) Cameras(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(l.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "camera-") {
			ids = append(ids, strings.TrimPrefix(e.Name(), "camera-"))
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (l *Local) writeMetadata(info SegmentInfo) error {
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(l.cameraDir(info.CameraID), info.FileName+".json"), b, 0o644)
}

func (l *Local) readMetadata(cameraID, fileName string) (SegmentInfo, error) {
	b, err := os.ReadFile(filepath.Join(l.cameraDir(cameraID), fileName+".json"))
	if err != nil {
		return SegmentInfo{}, err
	}
	var info SegmentInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return SegmentInfo{}, err
	}
	info.DownloadURL = fmt.Sprintf("/videos/%s/%s", info.CameraID, info.FileName)
	return info, nil
}

func (l *Local) cameraDir(cameraID string) string {
	return filepath.Join(l.root, "camera-"+safeName(cameraID))
}

func (l *Local) tmpDir() string {
	return filepath.Join(l.root, ".tmp")
}

func safeName(v string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		if r >= 'a' && r <= 'z' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r
		}
		if r == '-' || r == '_' {
			return r
		}
		return '-'
	}, v)
}

func validFileName(v string) bool {
	return v != "" && !strings.Contains(v, "/") && !strings.Contains(v, "\\") && strings.HasSuffix(v, ".mp4")
}

func (p *localPending) asPending() *PendingSegment {
	ps := p.PendingSegment
	ps.Writer = &localSegmentWriter{SegmentWriter: p.Writer, tmpPath: p.tmpPath, finalPath: p.finalPath}
	return &ps
}

type localSegmentWriter struct {
	SegmentWriter
	tmpPath   string
	finalPath string
}

func localFromPending(p *PendingSegment) (*localPending, error) {
	w, ok := p.Writer.(*localSegmentWriter)
	if !ok {
		return nil, fmt.Errorf("storage: pending segment is not local")
	}
	return &localPending{
		PendingSegment: *p,
		tmpPath:        w.tmpPath,
		finalPath:      w.finalPath,
	}, nil
}
