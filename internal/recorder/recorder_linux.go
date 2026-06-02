//go:build linux

package recorder

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/harnyk/tran/internal/config"
	"github.com/harnyk/tran/internal/meeting"
)

type Recorder struct {
	cfg      *config.Config
	monitor  string
	mic      string
	duration time.Duration
}

func New(cfg *config.Config) *Recorder {
	return &Recorder{cfg: cfg}
}

// Outputs returns metadata about the files the recorder will produce.
// Only valid after Record() has been called (devices are detected there).
func (r *Recorder) Outputs(m *meeting.MeetingDir) []OutputInfo {
	return []OutputInfo{
		{File: "mic.mp3", Description: "Audio capture – " + r.mic},
		{File: "sys.mp3", Description: "Audio capture – " + r.monitor},
		{File: "record.mp4", Description: "Mixed audio + screen capture " + r.cfg.Display},
	}
}

// Duration returns the recording duration. Only valid after Record() returns.
func (r *Recorder) Duration() time.Duration {
	return r.duration
}

// Record starts ffmpeg screen+audio recording. Blocks until ctx is cancelled.
// Sends SIGINT to ffmpeg on cancellation so it can flush files cleanly.
// Produces three simultaneous outputs in m.SourceDir():
//   - record.mp4: H.264 video + mixed audio archive
//   - mic.mp3:    raw microphone (high quality)
//   - sys.mp3:    raw system audio (high quality)
func (r *Recorder) Record(ctx context.Context, m *meeting.MeetingDir) error {
	monitor, mic, err := detectPulseAudioSources()
	if err != nil {
		return fmt.Errorf("detect PulseAudio sources: %w", err)
	}
	r.monitor = monitor
	r.mic = mic

	if err := os.MkdirAll(m.SourceDir(), 0755); err != nil {
		return fmt.Errorf("create source dir: %w", err)
	}

	args := []string{
		"-f", "x11grab",
		"-framerate", "30",
		"-i", r.cfg.Display,

		"-f", "pulse",
		"-i", monitor,

		"-f", "pulse",
		"-i", mic,

		// Mix system + mic for the video archive
		"-filter_complex", "[1:a][2:a]amix=inputs=2:normalize=0[mix]",

		// Output 1: video archive (video + mixed audio)
		"-map", "0:v",
		"-map", "[mix]",
		"-c:v", "libx264", "-crf", "28", "-preset", "ultrafast",
		"-vf", "scale=1280:-2", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart",
		"-y",
		m.RecordMP4Path(),

		// Output 2: mic audio (high quality MP3)
		"-map", "2:a",
		"-c:a", "libmp3lame", "-q:a", "0", "-ac", "1",
		"-y",
		m.MicMP3Path(),

		// Output 3: system audio (high quality MP3)
		"-map", "1:a",
		"-c:a", "libmp3lame", "-q:a", "0", "-ac", "1",
		"-y",
		m.SysMP3Path(),
	}

	cmd := exec.Command(r.cfg.FFmpegBin, args...)
	var ffmpegOutput bytes.Buffer
	cmd.Stdout = &ffmpegOutput
	cmd.Stderr = &ffmpegOutput
	// Run ffmpeg in its own process group so the terminal's Ctrl+C SIGINT
	// doesn't reach it directly — we send the single clean SIGINT ourselves.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	startedAt := time.Now()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		// Graceful stop: SIGINT lets ffmpeg flush all output files cleanly
		_ = cmd.Process.Signal(os.Interrupt)
		<-done
		r.duration = time.Since(startedAt)
		return nil
	case err := <-done:
		r.duration = time.Since(startedAt)
		if err != nil {
			return fmt.Errorf("ffmpeg exited with error: %w\n%s", err, ffmpegOutput.String())
		}
		return nil
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
