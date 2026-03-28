package converter

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/harnyk/tranny/internal/config"
	"github.com/harnyk/tranny/internal/meeting"
)

// segmentTime is computed as 23MB * 8 bits / 192kbps = 963 seconds
const segmentTime = 963

type Converter struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Converter {
	return &Converter{cfg: cfg}
}

type Result struct {
	Segments []string // absolute paths, sorted
}

// Convert extracts the mix audio track from the recording and segments it into MP3s.
func (c *Converter) Convert(ctx context.Context, m *meeting.MeetingDir) (*Result, error) {
	inputPath, err := m.RecordingPath()
	if err != nil {
		return nil, err
	}

	audioMap, err := m.AudioMapForFFmpeg()
	if err != nil {
		return nil, err
	}

	args := []string{
		"-i", inputPath,
		"-map", audioMap,
		"-vn",
		"-ar", "44100",
		"-ac", "1",
		"-b:a", "192k",
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", segmentTime),
		"-segment_start_number", "1",
		"-y",
		m.MP3Pattern(),
	}

	cmd := exec.CommandContext(ctx, c.cfg.FFmpegBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg convert: %w", err)
	}

	segments, err := m.ListMP3s()
	if err != nil {
		return nil, fmt.Errorf("list mp3s: %w", err)
	}
	return &Result{Segments: segments}, nil
}
