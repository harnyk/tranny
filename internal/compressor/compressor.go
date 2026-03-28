package compressor

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/harnyk/tranny/internal/config"
	"github.com/harnyk/tranny/internal/meeting"
	"github.com/harnyk/tranny/internal/recorder"
)

type Compressor struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Compressor {
	return &Compressor{cfg: cfg}
}

// Compress re-encodes the recording to a smaller H.264/AAC MP4 with only the mix audio track.
func (c *Compressor) Compress(ctx context.Context, m *meeting.MeetingDir) (string, error) {
	inputPath, err := m.RecordingPath()
	if err != nil {
		return "", err
	}

	profile, err := recorder.GetProfile(m.ProfileName)
	if err != nil {
		return "", err
	}

	out := m.CompressedMP4Path()

	args := []string{
		"-i", inputPath,
		"-map", "0:v:0",
		"-map", profile.AudioMap,
		"-c:v", "libx264",
		"-profile:v", "main",
		"-crf", "28",
		"-preset", "ultrafast",
		"-vf", "scale=1280:-2",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
		"-y",
		out,
	}

	cmd := exec.CommandContext(ctx, c.cfg.FFmpegBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg compress: %w", err)
	}
	return out, nil
}
