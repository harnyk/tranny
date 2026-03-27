# tranny

Go CLI for meeting-centric recording and transcription.

## Module

`github.com/harnyk/tranny` — Go 1.25, two external deps: `cobra`, `godotenv`.

## Commands

- `tranny rec [name]` — start ffmpeg recording, creates `YYYY-MM-DD-HH-MM-SS--Name/`
- `tranny mp3` — run from meeting dir, extracts `mix` track → `record-001.mp3`, ...
- `tranny transcript` — run from meeting dir, calls Whisper API → `transcript.txt`
- `tranny process` — run from meeting dir, detects and runs missing steps

## Meeting directory format

```
2026-03-27-16-30-00--Hr-interview-with-Alena/
  record.mkv
  record-001.mp3
  record-002.mp3
  transcript.txt
```

Time uses hyphens (not colons) for cross-tool compatibility.

## Package layout

```
cmd/         — cobra command wiring only, no business logic
internal/
  config/    — Config struct, loads ~/.config/tranny/config then env vars
  meeting/   — MeetingDir type, slug, detection, path helpers
  recorder/  — ffmpeg screen+audio subprocess (SIGINT for clean MKV close)
  converter/ — ffmpeg MKV → segmented MP3 (963s segments, 1-indexed)
  transcriber/ — OpenAI Whisper via raw net/http + mime/multipart
  format/    — formatTimestamp() helper
```

## Config

`~/.config/tranny/config` (godotenv format):
```
OPENAI_API_KEY=sk-...
OPENAI_MODEL_STT=whisper-1
FFMPEG_BIN=ffmpeg
DISPLAY=:0
```
Env vars override file values.

## Build & install

```bash
go build ./...
make install   # installs to $GOPATH/bin/tranny
```

## Key decisions

- Raw `net/http` for Whisper API (no OpenAI SDK) — full control, minimal deps
- SIGINT (not SIGKILL) to ffmpeg on stop — lets it flush the MKV moov atom
- `meeting.Detect()` looks for `record.mkv` in cwd — mp3/transcript/process commands must be run from inside the meeting dir
- Audio >25MB is compressed to temp file (64kbps mono 16kHz) before upload
