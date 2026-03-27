package recorder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/harnyk/tranny/internal/config"
)

type Recorder struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Recorder {
	return &Recorder{cfg: cfg}
}

// Record starts ffmpeg screen+audio recording. Blocks until ctx is cancelled.
// Sends SIGINT to ffmpeg on cancellation so it can flush the MKV moov atom cleanly.
func (r *Recorder) Record(ctx context.Context, outputPath string) error {
	monitor, mic, err := detectPulseAudioSources()
	if err != nil {
		return fmt.Errorf("detect PulseAudio sources: %w", err)
	}

	args := []string{
		"-f", "x11grab",
		"-framerate", "30",
		"-i", r.cfg.Display,

		"-f", "pulse",
		"-i", monitor,

		"-f", "pulse",
		"-i", mic,

		// Mix system + mic audio
		"-filter_complex", "[1:a][2:a]amix=inputs=2:normalize=0[mix]",
		"-map", "0:v",
		"-map", "1:a",
		"-map", "2:a",
		"-map", "[mix]",

		// Video codec
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",

		// Audio codec
		"-c:a", "aac", "-b:a", "160k",

		// Track metadata
		"-metadata:s:a:0", "title=system",
		"-metadata:s:a:1", "title=mic",
		"-metadata:s:a:2", "title=mix",
		"-disposition:a:0", "0",
		"-disposition:a:1", "0",
		"-disposition:a:2", "default",

		"-y",
		outputPath,
	}

	cmd := exec.Command(r.cfg.FFmpegBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		// Graceful stop: SIGINT lets ffmpeg flush the moov atom
		_ = cmd.Process.Signal(os.Interrupt)
		<-done
		return nil
	case err := <-done:
		return err
	}
}

func detectPulseAudioSources() (monitor, mic string, err error) {
	sink, err := runCmd("pactl", "get-default-sink")
	if err != nil {
		return "", "", fmt.Errorf("get default sink: %w", err)
	}
	monitor = strings.TrimSpace(sink) + ".monitor"

	src, err := runCmd("pactl", "get-default-source")
	if err != nil {
		return "", "", fmt.Errorf("get default source: %w", err)
	}
	mic = strings.TrimSpace(src)
	return monitor, mic, nil
}

func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
