//go:build darwin

package recorder

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/harnyk/tran/internal/config"
	"github.com/harnyk/tran/internal/meeting"
)

type Recorder struct {
	cfg      *config.Config
	duration time.Duration
}

func New(cfg *config.Config) *Recorder {
	return &Recorder{cfg: cfg}
}

// Outputs returns the audio files this recorder produces.
// On macOS, screen recording is not available; only audio files are produced.
func (r *Recorder) Outputs(m *meeting.MeetingDir) []OutputInfo {
	return []OutputInfo{
		{File: "mic.mp3", Description: "Audio capture – avfoundation device :" + r.cfg.MicDeviceIndex},
		{File: "sys.mp3", Description: "Audio capture – audiotee (system audio)"},
	}
}

// Duration returns the recording duration. Only valid after Record() returns.
func (r *Recorder) Duration() time.Duration {
	return r.duration
}

// Record captures microphone and system audio simultaneously.
// Blocks until ctx is cancelled. Sends SIGINT to all child processes on
// cancellation so they can flush output files cleanly.
//
// Produces two files in m.SourceDir():
//   - mic.mp3:  microphone audio via ffmpeg avfoundation
//   - sys.mp3:  system audio via audiotee piped to ffmpeg
func (r *Recorder) Record(ctx context.Context, m *meeting.MeetingDir) error {
	if err := checkAudiotee(r.cfg.AudioTeeBin); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "warning: macOS does not support screen recording; producing audio only (mic.mp3, sys.mp3)")

	if err := os.MkdirAll(m.SourceDir(), 0755); err != nil {
		return fmt.Errorf("create source dir: %w", err)
	}

	// sys pipeline: audiotee --stereo | ffmpeg -> sys.mp3
	audioteeCmd := exec.Command(r.cfg.AudioTeeBin, "--stereo", "--sample-rate", "48000")
	audioteeCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	audioteeCmd.Stderr = nil // suppress audiotee status messages

	sysOut, err := audioteeCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("audiotee stdout pipe: %w", err)
	}

	var ffSysOutput bytes.Buffer
	ffSysCmd := exec.Command(r.cfg.FFmpegBin,
		"-f", "s16le", "-ar", "48000", "-ac", "2", "-i", "pipe:0",
		"-c:a", "libmp3lame", "-q:a", "0", "-ac", "1",
		"-y", m.SysMP3Path(),
	)
	ffSysCmd.Stdin = sysOut
	ffSysCmd.Stdout = &ffSysOutput
	ffSysCmd.Stderr = &ffSysOutput
	ffSysCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// mic pipeline: ffmpeg avfoundation -> mic.mp3
	var ffMicOutput bytes.Buffer
	ffMicCmd := exec.Command(r.cfg.FFmpegBin,
		"-f", "avfoundation",
		"-i", ":"+r.cfg.MicDeviceIndex,
		"-c:a", "libmp3lame", "-q:a", "0", "-ac", "1",
		"-y", m.MicMP3Path(),
	)
	ffMicCmd.Stdout = &ffMicOutput
	ffMicCmd.Stderr = &ffMicOutput
	ffMicCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Start all processes before launching goroutines.
	if err := audioteeCmd.Start(); err != nil {
		return fmt.Errorf("start audiotee: %w", err)
	}
	if err := ffSysCmd.Start(); err != nil {
		_ = syscall.Kill(-audioteeCmd.Process.Pid, syscall.SIGKILL)
		return fmt.Errorf("start ffmpeg (sys): %w", err)
	}
	if err := ffMicCmd.Start(); err != nil {
		_ = syscall.Kill(-audioteeCmd.Process.Pid, syscall.SIGKILL)
		_ = syscall.Kill(-ffSysCmd.Process.Pid, syscall.SIGKILL)
		return fmt.Errorf("start ffmpeg (mic): %w", err)
	}

	startedAt := time.Now()

	// gctx is cancelled when ctx is cancelled OR when any goroutine returns error.
	eg, gctx := errgroup.WithContext(ctx)

	// Watcher: sends SIGINT to all process groups when gctx is done.
	eg.Go(func() error {
		<-gctx.Done()
		for _, pid := range []int{audioteeCmd.Process.Pid, ffSysCmd.Process.Pid, ffMicCmd.Process.Pid} {
			_ = syscall.Kill(-pid, syscall.SIGINT)
		}
		return nil
	})

	eg.Go(func() error {
		if err := audioteeCmd.Wait(); err != nil && gctx.Err() == nil {
			return fmt.Errorf("audiotee exited unexpectedly: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		if err := ffSysCmd.Wait(); err != nil && gctx.Err() == nil {
			return fmt.Errorf("ffmpeg (sys) exited with error: %w\n%s", err, ffSysOutput.String())
		}
		return nil
	})

	eg.Go(func() error {
		if err := ffMicCmd.Wait(); err != nil && gctx.Err() == nil {
			return fmt.Errorf("ffmpeg (mic) exited with error: %w\n%s", err, ffMicOutput.String())
		}
		return nil
	})

	// Block until parent ctx is cancelled (Ctrl+C) or a process crashes.
	<-gctx.Done()

	if err := eg.Wait(); err != nil {
		r.duration = time.Since(startedAt)
		return err
	}
	r.duration = time.Since(startedAt)
	return nil
}

// checkAudiotee verifies that the audiotee binary exists and is executable.
// Returns a descriptive error with build instructions if not found.
func checkAudiotee(bin string) error {
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf(
			"audiotee not found (looked for %q).\n"+
				"To build it:\n"+
				"  git clone --depth 1 https://github.com/makeusabrew/audiotee.git\n"+
				"  cd audiotee\n"+
				"  swift build -c release\n"+
				"Then set AUDIOTEE_BIN=/path/to/audiotee/.build/release/audiotee in ~/.config/tran/config",
			bin,
		)
	}
	return nil
}
