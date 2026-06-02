# tran

Repo notes for coding agents working in this project.

## Overview

- Module: `github.com/harnyk/tran`
- Language: Go
- CLI framework: `cobra`
- Config loading: `godotenv`-style file plus environment overrides
- Main purpose: record meetings, prepare audio, and transcribe them with OpenAI

## Current CLI surface

- `tran rec [meeting name]`
  Creates a timestamped meeting directory with `source/`, `mix/`, and `transcript/`.
  Records:
  - `source/record.mp4`: screen + mixed audio
  - `source/mic.mp3`: microphone channel
  - `source/sys.mp3`: system audio channel

- `tran soundmix`
  Reads `source/mic.mp3` and `source/sys.mp3`, applies optional per-channel gain,
  mixes them together, and writes segmented MP3 chunks to `mix/record-%03d.mp3`.

- `tran transcript`
  Default mode transcribes `mix/record-*.mp3` and writes `transcript/transcript.txt`.

- `tran transcript --dual-channel`
  Skips `mix/`, segments `source/mic.mp3` and `source/sys.mp3` separately, writes:
  - `transcript/transcript_mic.yaml`
  - `transcript/transcript_sys.yaml`
  Then merges them into `transcript/transcript.txt` with `Us` / `Them` labels.

- `tran process`
  Auto-runs missing steps.
  - default path: `soundmix` -> `transcript`
  - dual-channel path: `transcript --dual-channel`

## Meeting directory layout

```text
YYYY-MM-DD-HH-MM-SS--meeting-name/
  source/
    record.mp4
    mic.mp3
    sys.mp3
  mix/
    record-001.mp3
    record-002.mp3
    ...
  transcript/
    transcript.txt
    transcript_mic.yaml
    transcript_sys.yaml
```

Notes:

- `mix/` is only used by the mixed-audio pipeline.
- Commands that operate on a meeting expect to be run inside a directory that contains `source/`.

## Package layout

```text
cmd/                  cobra command wiring
internal/config/      config loading
internal/converter/   soundmix pipeline: mic + sys -> segmented MP3 chunks
internal/format/      timestamp formatting
internal/meeting/     meeting directory creation, detection, path helpers
internal/recorder/    ffmpeg recording orchestration
internal/transcriber/ OpenAI transcription client, dual-channel merge logic
```

## Important implementation details

- `meeting.Detect()` checks for `source/`, not a top-level media file.
- Mixed-audio chunking uses 192 kbps mono MP3 segments with `converter.SegmentTime == 963`.
- Files over the OpenAI upload limit are re-encoded to a temporary 64 kbps / 16 kHz mono MP3 before upload.
- Dual-channel transcript merging sorts segments by absolute timestamp and labels speakers as `Us` and `Them`.
- On macOS, `tran rec` uses `audiotee` (ScreenCaptureKit) for system audio and `ffmpeg -f avfoundation`
  for microphone. `record.mp4` is not produced. Screen Recording and Microphone permissions must be
  granted to the terminal that runs `tran`.

## Config

Config file: `~/.config/tran/config`

```env
OPENAI_API_KEY=sk-...
OPENAI_MODEL_STT=whisper-1
FFMPEG_BIN=ffmpeg
DISPLAY=:0

# macOS only
AUDIOTEE_BIN=/path/to/audiotee/.build/release/audiotee
AVFOUNDATION_MIC_INDEX=0
```

Environment variables override file values.
