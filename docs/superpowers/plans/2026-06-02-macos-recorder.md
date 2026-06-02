# macOS Recorder Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `tran rec` to macOS using `audiotee` (system audio via ScreenCaptureKit pipe) and `ffmpeg -f avfoundation` (microphone), producing `mic.mp3` + `sys.mp3` without screen recording.

**Architecture:** Two parallel ffmpeg pipelines run concurrently — `audiotee | ffmpeg` for system audio and `ffmpeg -f avfoundation` for microphone. Both are managed by a single `errgroup`; a watcher goroutine sends SIGINT to all processes when the shared context is cancelled. The existing Linux implementation moves to `recorder_linux.go`; the macOS implementation lives in `recorder_darwin.go`. The public `Recorder` API is unchanged.

**Tech Stack:** Go 1.25, `golang.org/x/sync/errgroup` (new dependency), `ffmpeg` (avfoundation input), `audiotee` (external Swift binary, user-supplied).

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `go.mod` / `go.sum` | Add `golang.org/x/sync` dependency |
| Modify | `internal/config/config.go` | Add `AudioTeeBin`, `MicDeviceIndex` fields |
| Create | `internal/recorder/recorder.go` | Shared `OutputInfo` type (no build tag) |
| Rename+modify | `internal/recorder/recorder_linux.go` (was `recorder.go`) | Add `//go:build linux` tag; remove `OutputInfo` definition |
| Create | `internal/recorder/recorder_darwin.go` | macOS Record() using audiotee + avfoundation |
| Modify | `cmd/rec.go` | Use `rec.Outputs(m)` for printing instead of hardcoded strings |
| Modify | `AGENTS.md` | Document new macOS config keys and behaviour |

---

## Task 1: Add `golang.org/x/sync` dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dependency**

```bash
go get golang.org/x/sync
```

Expected: `go.mod` now includes `golang.org/x/sync vX.Y.Z` under `require`.

- [ ] **Step 2: Verify the module builds**

```bash
go build ./...
```

Expected: exits 0, no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add golang.org/x/sync dependency"
```

---

## Task 2: Add macOS config fields

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add two fields to `Config`**

Replace the existing struct and `Load()` function with:

```go
type Config struct {
	OpenAIAPIKey    string
	STTModel        string
	FFmpegBin       string
	Display         string
	AudioTeeBin     string
	MicDeviceIndex  string
}

func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err == nil {
		cfgFile := filepath.Join(home, ".config", "tran", "config")
		vals, _ := godotenv.Read(cfgFile)
		for k, v := range vals {
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}

	cfg := &Config{
		OpenAIAPIKey:   os.Getenv("OPENAI_API_KEY"),
		STTModel:       envOrDefault("OPENAI_MODEL_STT", "whisper-1"),
		FFmpegBin:      envOrDefault("FFMPEG_BIN", "ffmpeg"),
		Display:        envOrDefault("DISPLAY", ":0"),
		AudioTeeBin:    envOrDefault("AUDIOTEE_BIN", "audiotee"),
		MicDeviceIndex: envOrDefault("AVFOUNDATION_MIC_INDEX", "0"),
	}
	return cfg, nil
}
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add AudioTeeBin and MicDeviceIndex config fields"
```

---

## Task 3: Split shared type and add build tag to Linux recorder

**Files:**
- Create: `internal/recorder/recorder.go` (new, no build tag)
- Rename+modify: `internal/recorder/recorder.go` → `internal/recorder/recorder_linux.go`

`OutputInfo` is currently defined inside `recorder.go`. Since both `recorder_linux.go` and `recorder_darwin.go` need it, move it to a new shared file with no build tag to avoid duplication.

- [ ] **Step 1: Rename the existing file**

```bash
git mv internal/recorder/recorder.go internal/recorder/recorder_linux.go
```

- [ ] **Step 2: Add the build constraint as the very first line of `recorder_linux.go`**

The file should start with (blank line required between constraint and package):

```go
//go:build linux

package recorder
```

Then remove the `OutputInfo` struct definition from `recorder_linux.go` (it will live in the shared file).

The `OutputInfo` block to remove:

```go
// OutputInfo describes a single output file produced by the recorder.
type OutputInfo struct {
	File        string
	Description string
}
```

- [ ] **Step 3: Create shared `internal/recorder/recorder.go`**

```go
package recorder

// OutputInfo describes a single output file produced by the recorder.
type OutputInfo struct {
	File        string
	Description string
}
```

- [ ] **Step 4: Verify Linux cross-compile**

```bash
GOOS=linux go build ./...
```

Expected: exits 0.

- [ ] **Step 5: Verify macOS build now fails (darwin impl missing)**

```bash
go build ./...
```

Expected: error like `undefined: recorder.Recorder` — confirms Task 4 is needed.

- [ ] **Step 6: Commit**

```bash
git add internal/recorder/recorder.go internal/recorder/recorder_linux.go
git commit -m "refactor: split OutputInfo to shared file, add linux build tag to recorder"
```

---

## Task 4: Implement macOS recorder

**Files:**
- Create: `internal/recorder/recorder_darwin.go`

- [ ] **Step 1: Create the file**

`OutputInfo` is NOT redefined here — it lives in `internal/recorder/recorder.go` (Task 3).

```go
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
```

- [ ] **Step 2: Verify macOS build**

```bash
go build ./...
```

Expected: exits 0, no errors.

- [ ] **Step 3: Verify Linux cross-compile still works**

```bash
GOOS=linux go build ./...
```

Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add internal/recorder/recorder_darwin.go
git commit -m "feat: add macOS recorder using audiotee + avfoundation"
```

---

## Task 5: Fix `cmd/rec.go` output listing

`cmd/rec.go` currently hardcodes three output files including `record.mp4`. On macOS this is wrong — `rec.Outputs(m)` already returns the correct platform-specific list. Update `rec.go` to use it.

**Files:**
- Modify: `cmd/rec.go`

- [ ] **Step 1: Replace hardcoded output lines**

Find this block in `cmd/rec.go` (lines 45–49):

```go
fmt.Println("Recording media:")
fmt.Printf("  %-12s  Audio capture (microphone)\n", "mic.mp3")
fmt.Printf("  %-12s  Audio capture (system monitor)\n", "sys.mp3")
fmt.Printf("  %-12s  Mixed audio + screen capture %s\n", "record.mp4", cfg.Display)
fmt.Println()
```

Replace with:

```go
fmt.Println("Recording media:")
for _, o := range rec.Outputs(m) {
    fmt.Printf("  %-12s  %s\n", o.File, o.Description)
}
fmt.Println()
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add cmd/rec.go
git commit -m "fix: use rec.Outputs() in rec command instead of hardcoded file list"
```

---

## Task 6: Update AGENTS.md

**Files:**
- Modify: `AGENTS.md`

- [ ] **Step 1: Add macOS config keys to the Config section**

Find the `## Config` section in `AGENTS.md`. Add the two new keys:

```env
OPENAI_API_KEY=sk-...
OPENAI_MODEL_STT=whisper-1
FFMPEG_BIN=ffmpeg
DISPLAY=:0

# macOS only
AUDIOTEE_BIN=/path/to/audiotee/.build/release/audiotee
AVFOUNDATION_MIC_INDEX=0
```

- [ ] **Step 2: Add a macOS note under `## Important implementation details`**

Add after the existing bullet points:

```markdown
- On macOS, `tran rec` uses `audiotee` (ScreenCaptureKit) for system audio and `ffmpeg -f avfoundation`
  for microphone. `record.mp4` is not produced. Screen Recording and Microphone permissions must be
  granted to the terminal that runs `tran`.
```

- [ ] **Step 3: Commit**

```bash
git add AGENTS.md
git commit -m "docs: document macOS recorder behaviour and config in AGENTS.md"
```

---

## Task 7: Manual smoke test on macOS

No automated tests exist for the recorder (requires real hardware). Verify manually:

- [ ] **Step 1: Ensure `audiotee` is built and reachable**

```bash
/path/to/audiotee/.build/release/audiotee --help
# or
audiotee --help
```

- [ ] **Step 2: Set config**

```bash
cat ~/.config/tran/config
# should contain AUDIOTEE_BIN and AVFOUNDATION_MIC_INDEX
```

- [ ] **Step 3: Run a short recording**

```bash
cd /tmp
tran rec smoketest
# wait 5 seconds
# press Ctrl+C
```

Expected output:
```
warning: macOS does not support screen recording; producing audio only (mic.mp3, sys.mp3)
Recording media:
  mic.mp3       Audio capture – avfoundation device :0
  sys.mp3       Audio capture – audiotee (system audio)
...
Recording started
Recording stopped (duration 0:05)
```

- [ ] **Step 4: Verify output files exist and are non-empty**

```bash
ls -lh /tmp/YYYY-MM-DD-*--smoketest/source/
```

Expected: `mic.mp3` and `sys.mp3` present, both > 0 bytes. No `record.mp4`.

- [ ] **Step 5: Verify audiotee-not-found error**

```bash
AUDIOTEE_BIN=/nonexistent tran rec test
```

Expected error:
```
audiotee not found (looked for "/nonexistent").
To build it:
  git clone --depth 1 https://github.com/makeusabrew/audiotee.git
  cd audiotee
  swift build -c release
Then set AUDIOTEE_BIN=/path/to/audiotee/.build/release/audiotee in ~/.config/tran/config
```
