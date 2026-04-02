# tranny

Go CLI for meeting-centric recording and transcription.

## Install

```bash
make install   # installs to $GOPATH/bin/tranny
```

Requires `ffmpeg` and `pactl` (PulseAudio) on `$PATH`.

## Config

`~/.config/tranny/config` (godotenv format):

```
OPENAI_API_KEY=sk-...
OPENAI_MODEL_STT=whisper-1
FFMPEG_BIN=ffmpeg
DISPLAY=:0
```

Environment variables override file values.

## Meeting directory layout

```
2026-03-27-16-30-00--Hr-interview-with-Alena/
  source/
    record.mp4      video archive (H.264, mixed audio)
    mic.mp3         raw microphone, lossless quality
    sys.mp3         raw system audio, lossless quality
  mix/
    record-001.mp3  normalised chunks for transcription
    record-002.mp3
  transcript/
    transcript.txt
```

## Commands

### `tranny rec [name]`

Start recording. Creates a timestamped meeting directory and launches a
single ffmpeg process that writes three files simultaneously to `source/`.
Stop with **Ctrl+C** — ffmpeg receives SIGINT and flushes all outputs cleanly.

```
tranny rec "HR interview with Alena"
```

### `tranny soundmix`

Run from the meeting directory. Reads `source/mic.mp3` and `source/sys.mp3`,
mixes them, and segments the result into `mix/record-NNN.mp3` chunks
(≤ 23 MB each, sized for the Whisper API 25 MB limit).

```
tranny soundmix                        # flat mix, no adjustments
tranny soundmix --mic-volume 6         # boost mic by +6 dB
tranny soundmix --sys-volume -3        # reduce system audio by 3 dB
tranny soundmix --mic-volume 6 --sys-volume -3
```

| Flag | Default | Description |
|---|---|---|
| `--mic-volume` | `0` | Microphone gain in dB |
| `--sys-volume` | `0` | System audio gain in dB |

### `tranny transcript`

Run from the meeting directory. Sends each `mix/record-NNN.mp3` chunk to the
OpenAI Whisper API and writes `transcript/transcript.txt` with segment-level
timestamps.

Files larger than 25 MB are automatically re-compressed to a 64 kbps / 16 kHz
mono temp file before upload.

Accepts `--lang` / `-l` to explicitly set the transcription language. Use an
ISO 639 language code with 2 or 3 letters like `en`, `eng`, or `pol`. The
default is `en`. Use `auto` to send an empty `language` parameter to the API.

```bash
tranny transcript --lang en
tranny transcript --lang eng
tranny transcript --lang auto
```

### `tranny process`

Run from the meeting directory. Detects and runs whichever steps are still
missing: soundmix → transcript.

```
tranny process                         # run missing steps with defaults
tranny process --mic-volume 6          # pass volume flags to soundmix step
tranny process --lang pl               # force Polish for the transcript step
tranny process --lang pol              # same, using a 3-letter ISO 639 code
tranny process --lang auto             # let the API auto-detect language
```

Accepts `--lang` / `-l` plus the same `--mic-volume` / `--sys-volume` flags as
`soundmix`.
Volume flags are only applied if the soundmix step actually runs (i.e. no
`mix/` chunks exist yet).

## Typical workflow

```bash
tranny rec "team standup"
# ... meeting happens, Ctrl+C to stop ...

cd 2026-04-02-*--team-standup/
tranny process
```

Or step by step:

```bash
tranny soundmix --mic-volume 3
tranny transcript --lang en
```

---

## Data flow diagrams

### `tranny rec` — ffmpeg graph

```mermaid
graph LR
    subgraph inputs["ffmpeg inputs"]
        X["x11grab\n:0  30 fps"]
        P1["PulseAudio\nsink.monitor\n(system audio)"]
        P2["PulseAudio\ndefault source\n(microphone)"]
    end

    subgraph filter["filter_complex"]
        AMIX["amix\ninputs=2\nnormalize=0"]
    end

    subgraph outputs["outputs  —  source/"]
        MP4["record.mp4\nlibx264 crf=28 ultrafast\nscale=1280:-2 yuv420p\naac 128k faststart"]
        MIC["mic.mp3\nlibmp3lame -q:a 0\nmono"]
        SYS["sys.mp3\nlibmp3lame -q:a 0\nmono"]
    end

    P1 --> AMIX
    P2 --> AMIX
    X  -->|"map 0:v"| MP4
    AMIX -->|"map [mix]"| MP4
    P2 -->|"map 2:a"| MIC
    P1 -->|"map 1:a"| SYS
```

### `tranny soundmix` — ffmpeg graph

```mermaid
graph LR
    subgraph inputs["ffmpeg inputs  —  source/"]
        MIC["mic.mp3"]
        SYS["sys.mp3"]
    end

    subgraph filter["filter_complex  (optional volume filters)"]
        MV["volume=XdB\n(--mic-volume)"]
        SV["volume=YdB\n(--sys-volume)"]
        AMIX["amix\ninputs=2\nnormalize=0"]
    end

    subgraph outputs["outputs  —  mix/"]
        SEG["record-001.mp3\nrecord-002.mp3\n…\n44.1 kHz mono 192k\n963 s chunks"]
    end

    MIC -->|"if --mic-volume ≠ 0"| MV
    MIC -->|"otherwise direct"| AMIX
    MV --> AMIX

    SYS -->|"if --sys-volume ≠ 0"| SV
    SYS -->|"otherwise direct"| AMIX
    SV --> AMIX

    AMIX --> SEG
```

### `tranny transcript` — chunk pipeline

```mermaid
graph LR
    subgraph mix["mix/"]
        C1["record-001.mp3"]
        C2["record-002.mp3"]
        CN["record-NNN.mp3"]
    end

    SIZE{"size\n> 25 MB?"}

    REENC["ffmpeg re-compress\n64 kbps mono 16 kHz\n(temp file)"]

    API["POST\napi.openai.com\n/v1/audio/transcriptions\nverbose_json + segment timestamps"]

    OUT["transcript/\ntranscript.txt"]

    C1 & C2 & CN --> SIZE
    SIZE -->|yes| REENC
    SIZE -->|no| API
    REENC --> API
    API -->|accumulate chunks| OUT
```

### Full pipeline

```mermaid
graph TD
    REC["tranny rec"]

    subgraph source["source/"]
        MP4["record.mp4"]
        MIC["mic.mp3"]
        SYS["sys.mp3"]
    end

    SM["tranny soundmix\n(--mic-volume / --sys-volume)"]

    subgraph mix["mix/"]
        CHUNKS["record-001.mp3 …"]
    end

    TR["tranny transcript"]

    subgraph transcript["transcript/"]
        TXT["transcript.txt"]
    end

    REC --> source
    MIC & SYS --> SM
    SM --> mix
    CHUNKS --> TR
    TR --> TXT
```
