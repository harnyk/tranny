package converter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

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

// Options controls optional volume adjustments applied before mixing.
// Values are in dB; 0 means no change.
type Options struct {
	MicVolumeDB float64
	SysVolumeDB float64
}

// buildFilterComplex returns the ffmpeg -filter_complex value for mixing mic
// and sys with optional per-channel volume adjustments.
func buildFilterComplex(opts Options) string {
	micIn := "0:a"
	sysIn := "1:a"
	var filters []string

	if opts.MicVolumeDB != 0 {
		filters = append(filters, fmt.Sprintf("[0:a]volume=%.4gdB[mic_v]", opts.MicVolumeDB))
		micIn = "mic_v"
	}
	if opts.SysVolumeDB != 0 {
		filters = append(filters, fmt.Sprintf("[1:a]volume=%.4gdB[sys_v]", opts.SysVolumeDB))
		sysIn = "sys_v"
	}
	filters = append(filters, fmt.Sprintf("[%s][%s]amix=inputs=2:normalize=0[mix]", micIn, sysIn))
	return strings.Join(filters, ";")
}

// Convert mixes mic and sys audio (with optional volume adjustments) and
// segments the result into 192 kbps mono MP3 chunks in mix/.
func (c *Converter) Convert(ctx context.Context, m *meeting.MeetingDir, opts Options) (*Result, error) {
	if err := os.MkdirAll(m.MixDir(), 0755); err != nil {
		return nil, fmt.Errorf("create mix dir: %w", err)
	}

	args := []string{
		"-i", m.MicMP3Path(),
		"-i", m.SysMP3Path(),
		"-filter_complex", buildFilterComplex(opts),
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
