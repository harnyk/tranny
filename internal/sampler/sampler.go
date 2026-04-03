package sampler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/harnyk/tran/internal/config"
	"github.com/harnyk/tran/internal/meeting"
)

const (
	maxSampleDuration = 10.0 // API limit in seconds
	silenceDB         = -30  // dBFS threshold
	silenceMinSecs    = 0.5  // minimum silence duration
)

type Sampler struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Sampler {
	return &Sampler{cfg: cfg}
}

type interval struct {
	start, end float64
}

func (iv interval) duration() float64 { return iv.end - iv.start }

// ExtractSamples creates sample/mic.sample.mp3 and sample/sys.sample.mp3
// using the longest non-silent segment found in each source file.
func (s *Sampler) ExtractSamples(ctx context.Context, m *meeting.MeetingDir) error {
	if err := os.MkdirAll(m.SampleDir(), 0755); err != nil {
		return fmt.Errorf("create sample dir: %w", err)
	}

	for _, track := range []struct {
		src, dst, label string
	}{
		{m.MicMP3Path(), m.MicSamplePath(), "mic"},
		{m.SysMP3Path(), m.SysSamplePath(), "sys"},
	} {
		seg, err := s.longestSpeechSegment(ctx, track.src)
		if err != nil {
			return fmt.Errorf("%s: %w", track.label, err)
		}
		dur := seg.duration()
		if dur > maxSampleDuration {
			dur = maxSampleDuration
		}
		fmt.Fprintf(os.Stderr, "Extracting %s sample: %.2fs–%.2fs (%.1fs speech) → %s\n",
			track.label, seg.start, seg.start+dur, dur, track.dst)
		if err := s.extract(ctx, track.src, seg.start, dur, track.dst); err != nil {
			return fmt.Errorf("%s: %w", track.label, err)
		}
	}
	return nil
}

// longestSpeechSegment finds the longest continuous non-silent interval in path.
func (s *Sampler) longestSpeechSegment(ctx context.Context, path string) (interval, error) {
	duration, err := s.fileDuration(ctx, path)
	if err != nil {
		return interval{}, err
	}

	args := []string{
		"-i", path,
		"-af", fmt.Sprintf("silencedetect=noise=%ddB:d=%.1f", silenceDB, silenceMinSecs),
		"-f", "null", "-",
	}
	cmd := exec.CommandContext(ctx, s.cfg.FFmpegBin, args...)
	out, _ := cmd.CombinedOutput()

	// Parse silence intervals
	type silInterval struct{ start, end float64 }
	var silences []silInterval
	var curStart float64
	hasCurStart := false

	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "silence_") {
			fmt.Fprintln(os.Stderr, "[silencedetect]", strings.TrimSpace(line))
		}
		parts := strings.Fields(line)
		for i, p := range parts {
			if i+1 >= len(parts) {
				continue
			}
			switch p {
			case "silence_start:":
				if t, err := strconv.ParseFloat(parts[i+1], 64); err == nil {
					curStart = t
					hasCurStart = true
				}
			case "silence_end:":
				if t, err := strconv.ParseFloat(parts[i+1], 64); err == nil && hasCurStart {
					silences = append(silences, silInterval{curStart, t})
					hasCurStart = false
				}
			}
		}
	}
	// Handle trailing silence that never got a silence_end
	if hasCurStart {
		silences = append(silences, silInterval{curStart, duration})
	}

	// Invert silence intervals → speech intervals
	var speech []interval
	pos := 0.0
	for _, sil := range silences {
		if sil.start > pos {
			speech = append(speech, interval{pos, sil.start})
		}
		pos = sil.end
	}
	if pos < duration {
		speech = append(speech, interval{pos, duration})
	}

	if len(speech) == 0 {
		return interval{}, fmt.Errorf("no speech segments found (try adjusting silence threshold)")
	}

	// Return the longest speech segment
	best := speech[0]
	for _, iv := range speech[1:] {
		if iv.duration() > best.duration() {
			best = iv
		}
	}
	return best, nil
}

// fileDuration returns the duration of an audio file in seconds via ffprobe.
func (s *Sampler) fileDuration(ctx context.Context, path string) (float64, error) {
	out, err := exec.CommandContext(ctx,
		"ffprobe", "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return d, nil
}

// extract cuts dur seconds starting at startSecs from src into dst.
func (s *Sampler) extract(ctx context.Context, src string, startSecs, dur float64, dst string) error {
	args := []string{
		"-ss", strconv.FormatFloat(startSecs, 'f', 3, 64),
		"-i", src,
		"-t", strconv.FormatFloat(dur, 'f', 3, 64),
		"-c:a", "libmp3lame", "-q:a", "4",
		"-y",
		dst,
	}
	cmd := exec.CommandContext(ctx, s.cfg.FFmpegBin, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
