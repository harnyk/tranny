# STT providers

`tran` supports four speech-to-text backends. Pick one via `STT_PROVIDER` in
`~/.config/tran/config`.

**STT config is only required for transcription.** These commands work without
`STT_PROVIDER` or API keys:

- `tran rec`
- `tran soundmix`
- `tran devices list` (macOS)

These commands **do** require a configured provider:

- `tran transcript`
- `tran process` (when the transcript step runs)

Environment variables override file values.

## Quick comparison

| Provider | Value | Where it runs | Needs network | Best for |
|----------|-------|---------------|---------------|----------|
| OpenAI | `openai` | Cloud | Yes | Default, highest quality on long meetings |
| Groq | `groq` | Cloud | Yes | Faster/cheaper cloud Whisper |
| whisper.cpp | `whispercpp` | Local | No | Offline on any OS with a ggml model |
| MLX Whisper | `mlx` | Local (Apple Silicon) | First run only | Offline on macOS without API keys |

Cloud providers (`openai`, `groq`) re-encode audio chunks over 25 MB to a
temporary 64 kbps / 16 kHz mono MP3 before upload.

---

## OpenAI

```env
STT_PROVIDER=openai
OPENAI_API_KEY=sk-...
OPENAI_MODEL_STT=whisper-1
```

Get an API key at [platform.openai.com](https://platform.openai.com/).

**Smoke test:**

```bash
cd integration-test/assets/meeting
tran transcript --dual-channel -l en
```

**Integration test:**

```bash
task test:integration:openai
```

Requires `OPENAI_API_KEY` in config or environment.

---

## Groq

Groq exposes an OpenAI-compatible Whisper API.

```env
STT_PROVIDER=groq
GROQ_API_KEY=gsk_...
GROQ_MODEL_STT=whisper-large-v3-turbo
```

Get a key at [console.groq.com/keys](https://console.groq.com/keys).

Usage is identical to OpenAI — same `tran transcript` / `tran process` commands.
Switch provider by changing `STT_PROVIDER`; no other CLI flags.

---

## whisper.cpp

Fully local. Requires the `whisper-cli` binary and a ggml model file.

```env
STT_PROVIDER=whispercpp
WHISPER_CPP_BIN=whisper-cli
WHISPER_MODEL_PATH=/path/to/ggml-large-v3-turbo.bin
```

**Install binary:**

```bash
brew install whisper-cpp
# or build from https://github.com/ggml-org/whisper.cpp
```

**Download a model** (inside the whisper.cpp repo):

```bash
./models/download-ggml-model.sh large-v3-turbo
```

**Smoke test:**

```bash
cd integration-test/assets/meeting
tran transcript --dual-channel -l en
```

---

## MLX Whisper (macOS only)

Local transcription on Apple Silicon via [uv](https://docs.astral.sh/uv/) /
`uvx`. No API key.

```env
STT_PROVIDER=mlx
UVX_BIN=uvx
MLX_WHISPER_MODEL=mlx-community/whisper-large-v3-turbo
```

**Install uv:**

```bash
curl -LsSf https://astral.sh/uv/install.sh | sh
```

The first `tran transcript` run downloads MLX packages through `uvx` (one-time).

**Smoke test:**

```bash
cd integration-test/assets/meeting
tran transcript --dual-channel -l en
```

**Integration test:**

```bash
task test:integration:mlx
```

---

## Switching providers

1. Edit `STT_PROVIDER` (and provider-specific keys) in `~/.config/tran/config`.
2. Re-run `tran transcript` or `tran process` from the meeting directory.

Intermediate YAML files (`transcript_mic.yaml`, `transcript_sys.yaml`) are
reused if they already exist. Delete `transcript/` first to force a full
re-transcription:

```bash
rm -rf transcript/
tran transcript --dual-channel -l en
```

## Language

All providers accept `--lang` / `-l` on `tran transcript` and `tran process`:

```bash
tran transcript --lang en
tran transcript --lang pol
tran transcript --lang auto    # empty language hint (provider decides)
```

## Test tasks

| Task | What it runs |
|------|--------------|
| `task test` | Unit tests |
| `task test:integration:openai` | Dual-channel e2e with OpenAI |
| `task test:integration:mlx` | Dual-channel e2e with MLX |
| `task test:integration` | All integration tests (each skips unless provider matches config) |
| `task test:all` | Unit + integration |

Regenerate TTS fixtures (requires OpenAI key for TTS API):

```bash
task test:integration:prepare
```
