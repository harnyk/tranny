# macOS Recorder Design

**Date:** 2026-06-02  
**Status:** Approved

## Summary

Port `tran rec` to macOS using `audiotee` (ScreenCaptureKit-based system audio tap) and `ffmpeg -f avfoundation` for microphone capture. Screen recording is not available on macOS; the command produces audio only (`mic.mp3` + `sys.mp3`).

## Goals

- `tran rec` works on macOS without changes to any command or downstream pipeline
- System audio captured via `audiotee` piped to `ffmpeg`
- Microphone captured via `ffmpeg -f avfoundation`
- Clear warning printed when screen recording is skipped
- Audiotee binary located via `AUDIOTEE_BIN` env var or `audiotee` in PATH; startup fails fast with a clear error if not found
- Microphone device configurable via `AVFOUNDATION_MIC_INDEX` (default `0` = system default)

## Non-Goals

- Screen recording (`record.mp4`) on macOS
- Auto-building `audiotee` from source
- Auto-detecting microphone device index via `ffmpeg -list_devices`

## Code Structure

Current `internal/recorder/recorder.go` is split into two platform-specific files. The public API is unchanged.

```
internal/recorder/
  recorder_linux.go   // go:build linux  — existing code, renamed
  recorder_darwin.go  // go:build darwin — new macOS implementation
```

Both files define the same `Recorder` struct and implement `New`, `Record`, `Outputs`, `Duration`. No changes to `cmd/` or any other package.

## Config Changes

Two new fields added to `config.Config` (compiled on all platforms; only used on darwin):

| Field            | Env var                  | Default     |
|------------------|--------------------------|-------------|
| `AudioTeeBin`    | `AUDIOTEE_BIN`           | `"audiotee"` |
| `MicDeviceIndex` | `AVFOUNDATION_MIC_INDEX` | `"0"`        |

`DISPLAY` field is retained and ignored on macOS.

Example `~/.config/tran/config` on macOS:
```env
OPENAI_API_KEY=sk-...
OPENAI_MODEL_STT=whisper-1
FFMPEG_BIN=ffmpeg
AUDIOTEE_BIN=/path/to/audiotee-src/.build/release/audiotee
AVFOUNDATION_MIC_INDEX=0
```

## recorder_darwin.go: Record() Logic

```
1. Verify cfg.AudioTeeBin exists and is executable; return descriptive error if not.
2. Print warning: "warning: macOS does not support screen recording; producing audio only (mic.mp3, sys.mp3)"
3. os.MkdirAll(m.SourceDir(), 0755)
4. Build two pipelines, each with Setpgid:true:

   sys pipeline:
     audiotee := exec.Command(cfg.AudioTeeBin, "--stereo", "--sample-rate", "48000")
     ffSys    := exec.Command(cfg.FFmpegBin,
                   "-f", "s16le", "-ar", "48000", "-ac", "2", "-i", "pipe:0",
                   "-c:a", "libmp3lame", "-q:a", "0", "-ac", "1", "-y",
                   m.SysMP3Path())
     ffSys.Stdin = audiotee pipe (StdoutPipe)

   mic pipeline:
     ffMic := exec.Command(cfg.FFmpegBin,
                "-f", "avfoundation",
                "-i", ":"+cfg.MicDeviceIndex,
                "-c:a", "libmp3lame", "-q:a", "0", "-ac", "1", "-y",
                m.MicMP3Path())

5. Start all three processes (audiotee, ffSys, ffMic).
6. Wait concurrently using golang.org/x/sync/errgroup (must be added to go.mod via `go get golang.org/x/sync`).
7. On ctx.Done(): send SIGINT to all three process groups; wait for errgroup.
8. If either ffmpeg exits non-zero before ctx is cancelled, cancel the other pipeline and return the error.
```

## Outputs() on darwin

Returns only two entries (no `record.mp4`):

```go
[]OutputInfo{
    {File: "mic.mp3", Description: "Audio capture – avfoundation device " + r.cfg.MicDeviceIndex},
    {File: "sys.mp3", Description: "Audio capture – audiotee (system audio)"},
}
```

## Error Handling

| Situation | Behaviour |
|-----------|-----------|
| `audiotee` not found / not executable | Return error before starting any process |
| `audiotee` exits before ctx cancelled | Cancel mic pipeline, return wrapped error |
| `ffMic` exits before ctx cancelled | Cancel sys pipeline, return wrapped error |
| Both exit cleanly after SIGINT | Return `nil` |

## Prerequisites for Users

- macOS 12.3+ (ScreenCaptureKit required by `audiotee`)
- `audiotee` built from source: `swift build -c release` in the `audiotee-src` directory
- `ffmpeg` installed (e.g. `brew install ffmpeg`)
- Screen Recording permission granted to Kitty (or whichever terminal runs `tran`)
- Microphone permission granted to Kitty

## AGENTS.md Updates

The `AGENTS.md` at the repo root should be updated to document:
- The macOS `AUDIOTEE_BIN` and `AVFOUNDATION_MIC_INDEX` config keys
- That `record.mp4` is not produced on macOS
