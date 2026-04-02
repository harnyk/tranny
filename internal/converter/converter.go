package converter

import (
	"bytes"
	"context"
	"encoding/json"
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

// loudnormMeasure holds the values produced by loudnorm's first analysis pass.
type loudnormMeasure struct {
	InputI      string `json:"input_i"`
	InputTP     string `json:"input_tp"`
	InputLRA    string `json:"input_lra"`
	InputThresh string `json:"input_thresh"`
}

// measureLoudnorm runs a loudnorm analysis pass on inputPath and returns the
// measured values needed for the linear (two-pass) normalization step.
func (c *Converter) measureLoudnorm(ctx context.Context, inputPath string) (*loudnormMeasure, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.cfg.FFmpegBin,
		"-i", inputPath,
		"-af", "loudnorm=I=-16:TP=-1.5:LRA=11:print_format=json",
		"-f", "null",
		"/dev/null",
	)
	cmd.Stderr = &stderr
	// ffmpeg exits non-zero when output is /dev/null; ignore the error.
	_ = cmd.Run()

	out := stderr.String()
	// The JSON block is printed at the very end of ffmpeg's stderr output.
	start := strings.LastIndex(out, "{")
	end := strings.LastIndex(out, "}") + 1
	if start == -1 || end <= start {
		return nil, fmt.Errorf("loudnorm JSON not found in ffmpeg output for %s", inputPath)
	}

	var m loudnormMeasure
	if err := json.Unmarshal([]byte(out[start:end]), &m); err != nil {
		return nil, fmt.Errorf("parse loudnorm output for %s: %w", inputPath, err)
	}
	return &m, nil
}

// loudnormFilter builds the two-pass (linear) loudnorm filter string for a stream.
func loudnormFilter(m *loudnormMeasure) string {
	return fmt.Sprintf(
		"loudnorm=I=-16:TP=-1.5:LRA=11:measured_I=%s:measured_TP=%s:measured_LRA=%s:measured_thresh=%s:linear=true",
		m.InputI, m.InputTP, m.InputLRA, m.InputThresh,
	)
}

// Convert normalises mic and sys audio independently to EBU R128 (-16 LUFS,
// -1.5 dB true-peak) using two-pass linear loudnorm, mixes them, and segments
// the result into 192 kbps mono MP3 chunks in mix/.
//
// Two-pass mode is used to avoid the dynamic gain artefacts of single-pass
// loudnorm (e.g. audio unexpectedly going silent after a loud event).
func (c *Converter) Convert(ctx context.Context, m *meeting.MeetingDir) (*Result, error) {
	if err := os.MkdirAll(m.MixDir(), 0755); err != nil {
		return nil, fmt.Errorf("create mix dir: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Measuring mic loudness...")
	micMeasure, err := c.measureLoudnorm(ctx, m.MicMP3Path())
	if err != nil {
		return nil, err
	}

	fmt.Fprintln(os.Stderr, "Measuring sys loudness...")
	sysMeasure, err := c.measureLoudnorm(ctx, m.SysMP3Path())
	if err != nil {
		return nil, err
	}

	filterComplex := fmt.Sprintf(
		"[0:a]%s[mic_norm];[1:a]%s[sys_norm];[mic_norm][sys_norm]amix=inputs=2:normalize=0[mix]",
		loudnormFilter(micMeasure),
		loudnormFilter(sysMeasure),
	)

	args := []string{
		"-i", m.MicMP3Path(),
		"-i", m.SysMP3Path(),
		"-filter_complex", filterComplex,
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
