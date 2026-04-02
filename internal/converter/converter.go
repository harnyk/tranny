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

// Convert normalizes mic and sys audio independently to EBU R128 (-16 LUFS, -1.5 dB true-peak),
// mixes them, and segments the result into 192kbps mono MP3 chunks in mix/.
func (c *Converter) Convert(ctx context.Context, m *meeting.MeetingDir) (*Result, error) {
	if err := os.MkdirAll(m.MixDir(), 0755); err != nil {
		return nil, fmt.Errorf("create mix dir: %w", err)
	}

	args := []string{
		"-i", m.MicMP3Path(),
		"-i", m.SysMP3Path(),
		"-filter_complex",
		"[0:a]loudnorm=I=-16:TP=-1.5:LRA=11[mic_norm];" +
			"[1:a]loudnorm=I=-16:TP=-1.5:LRA=11[sys_norm];" +
			"[mic_norm][sys_norm]amix=inputs=2:normalize=0[mix]",
		"-map", "[mix]",
		"-ar", "44100",
		"-ac", "1",
		"-b:a", "192k",
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", segmentTime),
		"-segment_start_number", "1",
		"-y",
		m.MixMP3Pattern(),
	}

	cmd := exec.CommandContext(ctx, c.cfg.FFmpegBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg convert: %w", err)
	}

	segments, err := m.ListMixMP3s()
	if err != nil {
		return nil, fmt.Errorf("list mp3s: %w", err)
	}
	return &Result{Segments: segments}, nil
}
