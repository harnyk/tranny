package recorder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/harnyk/tranny/internal/config"
)

type Recorder struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Recorder {
	return &Recorder{cfg: cfg}
}

// Record starts ffmpeg screen+audio recording using the given profile.
// Blocks until ctx is cancelled.
// Sends SIGINT to ffmpeg on cancellation so it can flush the moov atom cleanly.
func (r *Recorder) Record(ctx context.Context, outputPath string, profile *Profile) error {
	monitor, mic, err := detectPulseAudioSources()
	if err != nil {
		return fmt.Errorf("detect PulseAudio sources: %w", err)
	}

	params := RecordingParams{
		Display: r.cfg.Display,
		Monitor: monitor,
		Mic:     mic,
	}
	args := profile.BuildArgs(params, outputPath)

	cmd := exec.Command(r.cfg.FFmpegBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Run ffmpeg in its own process group so the terminal's Ctrl+C SIGINT
	// doesn't reach it directly — we send the single clean SIGINT ourselves.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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
