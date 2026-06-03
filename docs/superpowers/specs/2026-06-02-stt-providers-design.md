# STT Providers Design

**Date:** 2026-06-02  
**Status:** Approved

## Summary

Replace the hard-coded OpenAI Whisper HTTP client with a pluggable STT (speech-to-text) provider layer. Users must explicitly set `STT_PROVIDER`; there is no default. Four providers in v1: `openai`, `groq`, `whispercpp`, and `mlx` (macOS only, launched via `uvx`).

Downstream behavior is unchanged: segment timestamps, `transcript.txt` format, dual-channel YAML + Us/Them merge, chunking at `converter.SegmentTime`.

## Goals

- Support transcription without OpenAI API access
- Keep existing meeting pipeline and output formats
- Explicit configuration only (`STT_PROVIDER` required)
- Separate `groq` provider with dedicated env vars and error messages
- Local Apple Silicon path via `mlx` using `uvx` (fail fast if `uvx` missing)
- Local cross-platform path via `whisper.cpp` subprocess + JSON segments

## Non-Goals

- Default or auto-detected STT provider
- Bundling whisper.cpp binaries or MLX models in the `tran` release
- Real-time / streaming transcription
- Word-level timestamps in transcript output (segment-level only, as today)
- New cloud providers beyond `openai` and `groq` in v1
- Hugging Face Inference API as a hosted provider

## Breaking Change

`tran transcript` and `tran process` no longer infer OpenAI from `OPENAI_API_KEY` alone. Users must set:

```env
STT_PROVIDER=openai
OPENAI_API_KEY=sk-...
```

Existing configs that only set `OPENAI_API_KEY` will fail with a message listing valid providers.

## Architecture

### Package layout

```
internal/stt/
  provider.go          // Provider interface, Segment, Result, NewProvider factory
  openai.go            // openai + shared HTTP multipart client
  groq.go              // groq (wraps shared client, fixed base URL)
  whispercpp.go        // subprocess whisper-cli, parse JSON
  mlx.go               // go:build darwin — uvx mlx_whisper, parse JSON
  mlx_stub.go          // go:build !darwin — factory returns clear error
  openai_test.go       // multipart fields, response parsing
  whispercpp_test.go   // JSON fixture parsing
```

`internal/transcriber/transcriber.go` keeps orchestration (`TranscribeMeeting`, `TranscribeChannel`, merge, `prepareAudio` for cloud only). It holds `stt.Provider` instead of calling OpenAI directly.

### Interface

```go
type Segment struct {
    Start float64
    End   float64
    Text  string
}

type Result struct {
    Segments []Segment
}

type Provider interface {
    Transcribe(ctx context.Context, audioPath, language string) (*Result, error)
}
```

`language` is ISO 639-1 from `NormalizeLanguage` (e.g. `en`), or empty string for auto-detect.

### Factory

```go
func NewProvider(cfg *config.Config) (Provider, error)
```

| `STT_PROVIDER` | Implementation |
|----------------|----------------|
| `openai` | `openaiProvider` |
| `groq` | `groqProvider` |
| `whispercpp` | `whispercppProvider` |
| `mlx` | `mlxProvider` on darwin; error on other GOOS |

Empty or unknown value → error listing all four options and pointing to `~/.config/tran/config`.

## Config

New / changed fields in `config.Config`:

| Field | Env var | Used by | Default |
|-------|---------|---------|---------|
| `STTProvider` | `STT_PROVIDER` | all | *(none — required)* |
| `OpenAIAPIKey` | `OPENAI_API_KEY` | openai | — |
| `OpenAIModelSTT` | `OPENAI_MODEL_STT` | openai | `whisper-1` |
| `GroqAPIKey` | `GROQ_API_KEY` | groq | — |
| `GroqModelSTT` | `GROQ_MODEL_STT` | groq | `whisper-large-v3-turbo` |
| `WhisperCppBin` | `WHISPER_CPP_BIN` | whispercpp | `whisper-cli` |
| `WhisperModelPath` | `WHISPER_MODEL_PATH` | whispercpp | — |
| `UVXBin` | `UVX_BIN` | mlx | `uvx` |
| `MLXWhisperModel` | `MLX_WHISPER_MODEL` | mlx | `mlx-community/whisper-large-v3-turbo` |

`OPENAI_API_KEY` remains for integration-test TTS prep (`integration-test/prepare`); transcript step uses provider-specific keys.

### Example configs

**OpenAI (unchanged API, explicit provider):**

```env
STT_PROVIDER=openai
OPENAI_API_KEY=sk-...
OPENAI_MODEL_STT=whisper-1
```

**Groq (no OpenAI account):**

```env
STT_PROVIDER=groq
GROQ_API_KEY=gsk_...
GROQ_MODEL_STT=whisper-large-v3-turbo
```

**whisper.cpp (local, any OS with binary + model):**

```env
STT_PROVIDER=whispercpp
WHISPER_CPP_BIN=whisper-cli
WHISPER_MODEL_PATH=/path/to/ggml-large-v3-turbo.bin
```

**MLX on Apple Silicon (via uvx):**

```env
STT_PROVIDER=mlx
MLX_WHISPER_MODEL=mlx-community/whisper-large-v3-turbo
```

## Provider Implementations

### Shared: `openai` HTTP client

Used by `openai` and `groq`. Same multipart request as today:

- `model`, `language`, `file`
- `response_format=verbose_json`
- `timestamp_granularities[]=segment`

Parse response into `Result.Segments`.

`prepareAudio()` (25 MB re-encode) runs only for `openai` and `groq` before upload.

### `openai`

- URL: `https://api.openai.com/v1/audio/transcriptions`
- Auth: `Bearer {OPENAI_API_KEY}`
- Model: `OPENAI_MODEL_STT` (default `whisper-1`)
- Error if `OPENAI_API_KEY` empty

### `groq`

- URL: `https://api.groq.com/openai/v1/audio/transcriptions` (hard-coded)
- Auth: `Bearer {GROQ_API_KEY}`
- Model: `GROQ_MODEL_STT` (default `whisper-large-v3-turbo`)
- Error if `GROQ_API_KEY` empty; messages reference [Groq console](https://console.groq.com/keys) and rate limits

### `whispercpp`

1. Validate `WHISPER_MODEL_PATH` exists; `LookPath(WHISPER_CPP_BIN)`.
2. Optional: convert input to 16 kHz mono WAV in temp dir if CLI fails on MP3 (try MP3 first; WAV fallback only on failure).
3. Run:

   ```text
   whisper-cli -m {model} -f {audio} -l {lang|auto} -oj -of {tmpdir}/out -np
   ```

   Use `-l auto` when `language` is empty.

4. Read `{tmpdir}/out.json`, map `segments[].start` / `segments[].end` (float seconds, OpenAI-like `-oj` schema) to `Result.Segments`.

5. Startup error for missing binary should include build hint:

   ```text
   brew install whisper-cpp
   # or build from https://github.com/ggml-org/whisper.cpp
   ./models/download-ggml-model.sh large-v3-turbo
   ```

### `mlx` (darwin only)

**Requires `uvx` in PATH** (`UVX_BIN`, default `uvx`). If missing:

```text
STT_PROVIDER=mlx requires uvx (https://docs.astral.sh/uv/)
Install: curl -LsSf https://astral.sh/uv/install.sh | sh
```

No fallback to global `pip install mlx-whisper`.

**Command:**

```text
uvx --from mlx-whisper mlx_whisper \
  --model {MLX_WHISPER_MODEL} \
  --output-format json \
  --output-dir {tmpdir} \
  --output-name out \
  --verbose false \
  [--language {lang}] \
  {audioPath}
```

Omit `--language` when `language` is empty (auto-detect).

**Parse:** read `{tmpdir}/out.json` (mlx_whisper OpenAI-style export with `segments[]`). Map to `Result.Segments`.

**UX:** print once per process (or per first chunk): `Using mlx-whisper via uvx (first run may download packages)`.

**Constraints:**

- `GOOS != darwin` → `NewProvider` returns: `STT_PROVIDER=mlx is only supported on macOS; use whispercpp, groq, or openai`
- First `uvx` invocation needs network to install `mlx-whisper` and dependencies
- HF model weights download on first use (cached by mlx/huggingface)

### `mlx_stub` (!darwin)

`NewProvider` with `STT_PROVIDER=mlx` on Linux returns the macOS-only error above.

## Transcriber Changes

- `Transcriber` struct: add `provider stt.Provider`
- `transcriber.New(cfg)`: call `stt.NewProvider(cfg)`; propagate error to `cmd/root.go` at startup (fail fast on bad config)
- Replace `t.transcribeFile` body with `t.provider.Transcribe(ctx, path, language)`
- Remove direct `OPENAI_API_KEY` checks from `TranscribeMeeting` / `TranscribeMeetingDualChannel` (provider constructor validates)

## Error Handling

| Case | Behavior |
|------|----------|
| `STT_PROVIDER` unset | List providers + config path |
| Unknown provider | Same |
| Missing provider credentials / paths | Actionable message per provider |
| `mlx` without `uvx` | Install uv instructions |
| `mlx` on Linux | macOS-only message |
| Subprocess non-zero | Include stderr snippet |
| Empty segments | Treat as transcription error for that chunk |

## Testing

- **Unit:** `NewProvider` validation; groq/openai multipart field builder; whisper.cpp + mlx JSON parsing from committed fixtures
- **Integration:** extend skip logic — run when `STT_PROVIDER=openai` and `OPENAI_API_KEY` set (or add optional `groq` integration behind env); document manual smoke for `whispercpp` / `mlx`
- **CI:** no mlx/whisper.cpp binaries in GitHub Actions; provider tests use mocks/fixtures only

## Documentation Updates

- `README.md`: provider table, example configs, breaking change note
- `AGENTS.md`: `internal/stt`, required `STT_PROVIDER`, provider env vars
- Remove implication that OpenAI is the only transcription backend

## Prerequisites (user-facing)

| Provider | User installs |
|----------|----------------|
| openai | API key |
| groq | Free API key at console.groq.com |
| whispercpp | `whisper-cli` + ggml model file |
| mlx | `uv` (`uvx`), Apple Silicon Mac; model auto-downloaded |

## Future (out of v1)

- `STT_BASE_URL` override for custom OpenAI-compatible servers
- `faster-whisper` / `speaches` as `http-local` provider
- Optional word-level timestamps in output
