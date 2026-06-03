# macOS: prerequisites and limitations

`tran rec` on macOS captures **audio only** — microphone and system sound.
Downstream steps (`soundmix`, `transcript`, `process`) work the same as on
Linux.

## What works vs Linux

| | Linux | macOS |
|---|-------|-------|
| `source/mic.mp3` | PulseAudio default source | ffmpeg avfoundation |
| `source/sys.mp3` | PulseAudio sink monitor | audiotee (ScreenCaptureKit) |
| `source/record.mp4` | Yes (x11grab + H.264) | **No** |
| STT providers | all four | all except mlx requires macOS; mlx is macOS-only |

When you run `tran rec`, expect this warning:

```text
warning: macOS does not support screen recording; producing audio only (mic.mp3, sys.mp3)
```

Dual-channel transcription (`tran transcript --dual-channel`) is the natural
fit for macOS recordings: mic and system audio are already separate files.

---

## Prerequisites

### 1. macOS 12.3+

Required for ScreenCaptureKit (used by audiotee).

### 2. ffmpeg

```bash
brew install ffmpeg
```

Set in config if needed:

```env
FFMPEG_BIN=ffmpeg
```

### 3. audiotee (system audio)

`tran rec` pipes [audiotee](https://github.com/makeusabrew/audiotee) into ffmpeg
for system audio capture. The binary is **not** bundled — build it once:

```bash
git clone --depth 1 https://github.com/makeusabrew/audiotee.git
cd audiotee
swift build -c release
```

Point `tran` at the binary:

```env
AUDIOTEE_BIN=/path/to/audiotee/.build/release/audiotee
```

Or put `audiotee` on `$PATH`.

If audiotee is missing, `tran rec` fails immediately with build instructions.

### 4. Permissions

Grant the **terminal app** that runs `tran` (Terminal, iTerm, Cursor, etc.):

- **Screen Recording** — required by audiotee / ScreenCaptureKit for system audio
- **Microphone** — required by ffmpeg avfoundation for mic capture

System Settings → Privacy & Security → Screen Recording / Microphone.

If capture is silent, check permissions first.

### 5. STT (transcription only)

Recording does **not** need `STT_PROVIDER`. For `tran transcript` / `tran process`,
configure a provider — see [stt-providers.md](stt-providers.md).

Local option without API keys on Apple Silicon:

```env
STT_PROVIDER=mlx
```

---

## Microphone device selection

Microphone capture uses ffmpeg avfoundation with a **numeric device index**, not
the macOS system default.

```env
AVFOUNDATION_MIC_INDEX=1
```

List devices and see the configured index:

```bash
tran devices list
```

Or manually:

```bash
ffmpeg -f avfoundation -list_devices true -i "" 2>&1 | grep -A20 "audio devices"
```

### Bluetooth headphones

If BT headphones are connected, ffmpeg often lists them at **index 0**. Opening
that device as a microphone switches the headset to **Hands-Free Profile (HFP)** —
low-quality mono audio on both input and output.

**Recommended setup:**

- **Output:** BT headphones (A2DP stereo)
- **Input:** MacBook built-in microphone (`AVFOUNDATION_MIC_INDEX` = index of
  "MacBook … Microphone" from `tran devices list`)

Do **not** use the BT headset index unless you intentionally want hands-free
quality.

---

## Example config

```env
# Recording (macOS)
FFMPEG_BIN=ffmpeg
AUDIOTEE_BIN=/path/to/audiotee/.build/release/audiotee
AVFOUNDATION_MIC_INDEX=1

# Transcription (pick one provider — see stt-providers.md)
STT_PROVIDER=mlx
MLX_WHISPER_MODEL=mlx-community/whisper-large-v3-turbo
```

---

## Typical workflow

```bash
# 1. Record (no STT config needed)
tran rec "team sync"
# Ctrl+C when done

# 2. Transcribe with speaker labels
cd 2026-*--team-sync/
tran process --dual-channel -l en
```

Step by step:

```bash
tran transcript --dual-channel -l en
```

Output: `transcript/transcript.txt` with `Us` (mic) and `Them` (system audio)
labels.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `audiotee not found` | Binary not built or wrong path | Build audiotee, set `AUDIOTEE_BIN` |
| Empty / silent `sys.mp3` | Screen Recording permission denied | Grant permission, restart terminal |
| Empty / silent `mic.mp3` | Microphone permission denied | Grant permission, restart terminal |
| BT audio degrades on record start | Wrong mic index (BT HFP) | Set `AVFOUNDATION_MIC_INDEX` to built-in mic |
| `STT_PROVIDER is not set` | Running transcript without config | Add provider to config (not needed for `rec`) |
