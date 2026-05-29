package encoder

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

type Config struct {
	Path      string
	OutputFPS int
	Preset    string
	CRF       int
}

type FFmpeg struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	done   chan error
	cancel context.CancelFunc
	once   sync.Once
}

func Start(ctx context.Context, cfg Config, out io.Writer) (*FFmpeg, error) {
	ctx, cancel := context.WithCancel(ctx)
	args := args(cfg)
	cmd := exec.CommandContext(ctx, cfg.Path, args...)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open ffmpeg stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open ffmpeg stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(out, stdout)
		waitErr := cmd.Wait()
		if copyErr != nil {
			done <- fmt.Errorf("copy encoded mp4: %w", copyErr)
			return
		}
		if waitErr != nil {
			done <- fmt.Errorf("ffmpeg exited: %w", waitErr)
			return
		}
		done <- nil
	}()

	return &FFmpeg{cmd: cmd, stdin: stdin, done: done, cancel: cancel}, nil
}

func (e *FFmpeg) WriteJPEG(jpeg []byte) error {
	if len(jpeg) == 0 {
		return nil
	}
	if _, err := e.stdin.Write(jpeg); err != nil {
		return fmt.Errorf("write jpeg to ffmpeg: %w", err)
	}
	return nil
}

func (e *FFmpeg) Close() error {
	e.once.Do(func() {
		_ = e.stdin.Close()
	})
	return <-e.done
}

func (e *FFmpeg) Kill() {
	e.cancel()
	_ = e.stdin.Close()
	<-e.done
}

func args(cfg Config) []string {
	gop := cfg.OutputFPS * 2
	return []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-f", "mjpeg",
		"-framerate", strconv.Itoa(cfg.OutputFPS),
		"-i", "pipe:0",
		"-an",
		"-c:v", "libx264",
		"-preset", cfg.Preset,
		"-crf", strconv.Itoa(cfg.CRF),
		"-pix_fmt", "yuv420p",
		"-r", strconv.Itoa(cfg.OutputFPS),
		"-g", strconv.Itoa(gop),
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-f", "mp4",
		"pipe:1",
	}
}
