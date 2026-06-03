# STT Providers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add pluggable STT backends (`openai`, `groq`, `whispercpp`, `mlx`) with mandatory `STT_PROVIDER` config, preserving existing transcript output formats.

**Architecture:** New `internal/stt` package defines `Provider` + `NewProvider(cfg)`. Cloud providers share multipart HTTP logic; local providers run subprocesses and parse JSON segment output. `internal/transcriber` delegates `transcribeFile` to the provider; 25 MB re-encode stays in cloud providers only.

**Tech Stack:** Go 1.25, `net/http`, `os/exec`, `uvx` + `mlx-whisper` (darwin), `whisper-cli` (user-installed).

**Spec:** `docs/superpowers/specs/2026-06-02-stt-providers-design.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/config/config.go` | STT provider + per-provider env fields |
| Create | `internal/stt/provider.go` | Types, `NewProvider`, validation errors |
| Create | `internal/stt/openai.go` | Shared HTTP transcribe + `prepareUploadAudio` |
| Create | `internal/stt/groq.go` | Groq URL + key |
| Create | `internal/stt/whispercpp.go` | Subprocess + JSON parse |
| Create | `internal/stt/mlx.go` | `//go:build darwin` uvx mlx_whisper |
| Create | `internal/stt/mlx_stub.go` | `//go:build !darwin` mlx error |
| Create | `internal/stt/openai_test.go` | Multipart fields + HTTP response parse |
| Create | `internal/stt/parse_test.go` | Shared JSON segment parsing fixtures |
| Create | `internal/stt/testdata/verbose_segments.json` | Fixture for parsers |
| Modify | `internal/transcriber/transcriber.go` | Use `stt.Provider`; remove OpenAI-only code |
| Modify | `internal/transcriber/transcriber_test.go` | Drop moved multipart tests |
| Modify | `cmd/root.go` | Fail fast if `transcriber.New` errors |
| Modify | `integration-test/transcription_test.go` | Skip unless `STT_PROVIDER=openai` + key |
| Modify | `README.md`, `AGENTS.md` | Provider docs + breaking change |

---

## Task 1: Extend config for STT providers

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Replace `STTModel` with provider-specific fields**

```go
type Config struct {
	OpenAIAPIKey      string
	OpenAIModelSTT    string
	GroqAPIKey        string
	GroqModelSTT      string
	STTProvider       string
	FFmpegBin         string
	Display           string
	AudioTeeBin       string
	MicDeviceIndex    string
	WhisperCppBin     string
	WhisperModelPath  string
	UVXBin            string
	MLXWhisperModel   string
}

func Load() (*Config, error) {
	// ... existing godotenv block unchanged ...

	cfg := &Config{
		OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
		OpenAIModelSTT:   envOrDefault("OPENAI_MODEL_STT", "whisper-1"),
		GroqAPIKey:       os.Getenv("GROQ_API_KEY"),
		GroqModelSTT:     envOrDefault("GROQ_MODEL_STT", "whisper-large-v3-turbo"),
		STTProvider:      os.Getenv("STT_PROVIDER"),
		FFmpegBin:        envOrDefault("FFMPEG_BIN", "ffmpeg"),
		Display:          envOrDefault("DISPLAY", ":0"),
		AudioTeeBin:      envOrDefault("AUDIOTEE_BIN", "audiotee"),
		MicDeviceIndex:   envOrDefault("AVFOUNDATION_MIC_INDEX", "0"),
		WhisperCppBin:    envOrDefault("WHISPER_CPP_BIN", "whisper-cli"),
		WhisperModelPath: os.Getenv("WHISPER_MODEL_PATH"),
		UVXBin:           envOrDefault("UVX_BIN", "uvx"),
		MLXWhisperModel:  envOrDefault("MLX_WHISPER_MODEL", "mlx-community/whisper-large-v3-turbo"),
	}
	return cfg, nil
}
```

- [ ] **Step 2: Grep for old `STTModel` / `cfg.STTModel` and update call sites** (transcriber — fixed in Task 7)

```bash
rg 'STTModel' .
```

Expected: only `config.go` until Task 7.

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

Expected: may fail on `cfg.STTModel` in transcriber until Task 7 — if so, that's OK for this task only if you batch Tasks 1+7; otherwise apply Task 7 same commit.

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add STT provider config fields"
```

---

## Task 2: `internal/stt` core types and factory

**Files:**
- Create: `internal/stt/provider.go`

- [ ] **Step 1: Write `provider.go`**

```go
package stt

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/harnyk/tran/internal/config"
)

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

func NewProvider(cfg *config.Config) (Provider, error) {
	p := strings.TrimSpace(strings.ToLower(cfg.STTProvider))
	if p == "" {
		return nil, missingProviderError()
	}
	switch p {
	case "openai":
		return newOpenAIProvider(cfg)
	case "groq":
		return newGroqProvider(cfg)
	case "whispercpp":
		return newWhisperCppProvider(cfg)
	case "mlx":
		return newMLXProvider(cfg)
	default:
		return nil, fmt.Errorf("unknown STT_PROVIDER %q\n%s", cfg.STTProvider, providerHint())
	}
}

func missingProviderError() error {
	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, ".config", "tran", "config")
	return fmt.Errorf("STT_PROVIDER is not set\n%s\nSet STT_PROVIDER in %s", providerHint(), cfgPath)
}

func providerHint() string {
	return "Valid values: openai | groq | whispercpp | mlx (macOS only)"
}

func mlxUnsupportedGOOS() error {
	return fmt.Errorf("STT_PROVIDER=mlx is only supported on macOS (current: %s); use whispercpp, groq, or openai", runtime.GOOS)
}
```

Add `"context"` import.

- [ ] **Step 2: Write failing factory test**

Create `internal/stt/provider_test.go`:

```go
func TestNewProviderMissing(t *testing.T) {
	_, err := NewProvider(&config.Config{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "STT_PROVIDER") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewProviderUnknown(t *testing.T) {
	_, err := NewProvider(&config.Config{STTProvider: "azure"})
	if err == nil || !strings.Contains(err.Error(), "unknown STT_PROVIDER") {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 3: Run test**

```bash
go test ./internal/stt/... -run TestNewProvider -v
```

Expected: PASS for missing/unknown; openai/groq tests added in Task 3.

- [ ] **Step 4: Commit**

```bash
git add internal/stt/provider.go internal/stt/provider_test.go
git commit -m "feat: add stt Provider interface and factory"
```

---

## Task 3: OpenAI + Groq HTTP providers

**Files:**
- Create: `internal/stt/openai.go`
- Create: `internal/stt/groq.go`
- Create: `internal/stt/openai_test.go`
- Create: `internal/stt/testdata/verbose_segments.json`

- [ ] **Step 1: Add fixture `testdata/verbose_segments.json`**

```json
{
  "text": "hello world",
  "segments": [
    {"start": 0.0, "end": 1.5, "text": "hello"},
    {"start": 1.5, "end": 2.0, "text": "world"}
  ]
}
```

- [ ] **Step 2: Implement `openai.go`** (move logic from `transcriber.go`)

Key pieces:
- `const maxUploadBytes = 25 * 1024 * 1024`
- `type httpProvider struct { cfg *config.Config; url, apiKey, model, ffmpegBin string; client *http.Client }`
- `newOpenAIProvider` — validates `OPENAI_API_KEY`, sets url `https://api.openai.com/v1/audio/transcriptions`
- `newGroqProvider` in `groq.go` — validates `GROQ_API_KEY`, url `https://api.groq.com/openai/v1/audio/transcriptions`, model `cfg.GroqModelSTT`
- `writeTranscriptionFields(w, model, language)` — copy from transcriber
- `parseVerboseJSON(data []byte) (*Result, error)` — unmarshal segments, trim text
- `prepareUploadAudio(ffmpegBin, path string) (outPath string, cleanup func(), err error)` — copy `prepareAudio` from transcriber
- `(p *httpProvider) Transcribe(ctx, audioPath, language string) (*Result, error)` — prepare, multipart POST, parse

- [ ] **Step 3: Move tests to `openai_test.go`**

```go
func TestWriteTranscriptionFields(t *testing.T) { /* same cases as transcriber_test */ }

func TestParseVerboseJSON(t *testing.T) {
	data, _ := os.ReadFile("testdata/verbose_segments.json")
	res, err := parseVerboseJSON(data)
	// assert 2 segments, start/end/text
}
```

- [ ] **Step 4: Add provider validation tests**

```go
func TestNewOpenAIProviderMissingKey(t *testing.T) {
	_, err := NewProvider(&config.Config{STTProvider: "openai"})
	// want error containing OPENAI_API_KEY
}

func TestNewGroqProviderMissingKey(t *testing.T) {
	_, err := NewProvider(&config.Config{STTProvider: "groq"})
	// want error containing GROQ_API_KEY and console.groq.com
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/stt/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/stt/
git commit -m "feat: add openai and groq STT HTTP providers"
```

---

## Task 4: whisper.cpp subprocess provider

**Files:**
- Create: `internal/stt/whispercpp.go`
- Create: `internal/stt/parse.go` (shared `parseVerboseJSON` if not already in openai.go — export for reuse)
- Extend: `internal/stt/provider_test.go` or `parse_test.go`

- [ ] **Step 1: Implement `newWhisperCppProvider` validation**

- `LookPath(cfg.WhisperCppBin)`
- `os.Stat(cfg.WhisperModelPath)` must exist
- Errors mention `brew install whisper-cpp` and model download script

- [ ] **Step 2: Implement `Transcribe`**

```go
func (p *whisperCppProvider) Transcribe(ctx context.Context, audioPath, language string) (*Result, error) {
	tmpDir, err := os.MkdirTemp("", "tran-whispercpp-*")
	// defer os.RemoveAll(tmpDir)
	langFlag := "auto"
	if language != "" {
		langFlag = language
	}
	outBase := filepath.Join(tmpDir, "out")
	cmd := exec.CommandContext(ctx, p.bin,
		"-m", p.modelPath,
		"-f", audioPath,
		"-l", langFlag,
		"-oj", "-of", outBase, "-np",
	)
	// capture stderr; Run(); ReadFile(outBase+".json"); parseVerboseJSON
}
```

If `Run()` fails with MP3-related stderr, optionally convert via ffmpeg to 16 kHz mono WAV in `tmpDir` and retry once (same ffmpeg flags as transcriber `prepareAudio` without bitrate cap).

- [ ] **Step 3: Test JSON parse via fixture** (no subprocess in CI)

Uses `parseVerboseJSON` + `testdata/verbose_segments.json` in `parse_test.go`.

- [ ] **Step 4: Test missing model path**

```go
func TestNewWhisperCppMissingModel(t *testing.T) {
	_, err := NewProvider(&config.Config{
		STTProvider: "whispercpp",
		WhisperCppBin: "whisper-cli",
	})
	// expect WHISPER_MODEL_PATH error
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/stt/... -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/stt/whispercpp.go internal/stt/parse_test.go
git commit -m "feat: add whispercpp STT provider"
```

---

## Task 5: MLX provider (darwin + stub)

**Files:**
- Create: `internal/stt/mlx.go` (`//go:build darwin`)
- Create: `internal/stt/mlx_stub.go` (`//go:build !darwin`)

- [ ] **Step 1: `mlx_stub.go`**

```go
//go:build !darwin

package stt

import "github.com/harnyk/tran/internal/config"

func newMLXProvider(cfg *config.Config) (Provider, error) {
	return nil, mlxUnsupportedGOOS()
}
```

- [ ] **Step 2: `mlx.go` — require uvx**

```go
//go:build darwin

func newMLXProvider(cfg *config.Config) (Provider, error) {
	if _, err := exec.LookPath(cfg.UVXBin); err != nil {
		return nil, fmt.Errorf(
			"STT_PROVIDER=mlx requires uvx (https://docs.astral.sh/uv/)\n"+
				"Install: curl -LsSf https://astral.sh/uv/install.sh | sh",
		)
	}
	return &mlxProvider{uvx: cfg.UVXBin, model: cfg.MLXWhisperModel, ffmpegBin: cfg.FFmpegBin}, nil
}
```

- [ ] **Step 3: `Transcribe` via uvx**

```go
cmd := exec.CommandContext(ctx, p.uvx,
	"--from", "mlx-whisper", "mlx_whisper",
	"--model", p.model,
	"--output-format", "json",
	"--output-dir", tmpDir,
	"--output-name", "out",
	"--verbose", "false",
)
// append --language only if language != ""
// final arg: audioPath
```

Print once (package-level `sync.Once`):

```go
fmt.Println("Using mlx-whisper via uvx (first run may download packages)")
```

Read `filepath.Join(tmpDir, "out.json")` → `parseVerboseJSON`.

- [ ] **Step 4: Test stub on Linux CI**

```go
// provider_test.go with build tag !darwin
func TestMLXOnLinux(t *testing.T) {
	_, err := NewProvider(&config.Config{STTProvider: "mlx"})
	if !strings.Contains(err.Error(), "macOS") {
		t.Fatalf("got %v", err)
	}
}
```

On darwin dev machines, skip or manual.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/stt/... -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/stt/mlx.go internal/stt/mlx_stub.go
git commit -m "feat: add mlx STT provider via uvx on macOS"
```

---

## Task 6: Wire transcriber to `stt.Provider`

**Files:**
- Modify: `internal/transcriber/transcriber.go`
- Modify: `internal/transcriber/transcriber_test.go`
- Modify: `cmd/root.go`

- [ ] **Step 1: Change `Transcriber` struct and `New`**

```go
type Transcriber struct {
	cfg      *config.Config
	provider stt.Provider
}

func New(cfg *config.Config) (*Transcriber, error) {
	p, err := stt.NewProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &Transcriber{cfg: cfg, provider: p}, nil
}
```

- [ ] **Step 2: Replace `transcribeFile`**

```go
func (t *Transcriber) transcribeFile(ctx context.Context, audioPath string, language string) (*stt.Result, error) {
	result, err := t.provider.Transcribe(ctx, audioPath, language)
	if err != nil {
		return nil, err
	}
	if len(result.Segments) == 0 {
		return nil, fmt.Errorf("no segments in transcription result")
	}
	return result, nil
}
```

Update callers to use `result.Segments` instead of `whisperResponse`.

- [ ] **Step 3: Remove from transcriber.go**

- `whisperURL`, `whisperSegment`, `whisperResponse`
- `writeTranscriptionFields`, `prepareAudio`, direct `http.Client` on struct
- `OPENAI_API_KEY` checks in `TranscribeMeeting` / `TranscribeMeetingDualChannel`

- [ ] **Step 4: Update `cmd/root.go`**

```go
func initServices() {
	// ...
	trans, err = transcriber.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "transcriber:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Trim `transcriber_test.go`**

Remove `TestWriteTranscriptionFields` and `transcriptionFields` helper (moved to `stt`).

- [ ] **Step 6: Build and test**

```bash
go build ./...
go test ./...
```

Expected: all PASS (integration may skip).

- [ ] **Step 7: Commit**

```bash
git add internal/transcriber/ cmd/root.go
git commit -m "feat: delegate transcription to stt.Provider"
```

---

## Task 7: Integration test + docs

**Files:**
- Modify: `integration-test/transcription_test.go`
- Modify: `README.md`
- Modify: `AGENTS.md`

- [ ] **Step 1: Update integration skip logic**

```go
if cfg.STTProvider != "openai" || cfg.OpenAIAPIKey == "" {
	t.Skip("integration test requires STT_PROVIDER=openai and OPENAI_API_KEY")
}
```

- [ ] **Step 2: Update `README.md`**

Add section **Speech-to-text providers** with:
- Breaking change: `STT_PROVIDER` required
- Table of four providers + env vars
- Example configs (openai, groq, whispercpp, mlx)
- Groq signup link
- whisper.cpp + uvx install one-liners

- [ ] **Step 3: Update `AGENTS.md`**

- Package layout: `internal/stt/`
- Config table for all `STT_*` / provider env vars
- Note: no default provider

- [ ] **Step 4: Run full test suite**

```bash
go test ./...
```

- [ ] **Step 5: Commit**

```bash
git add integration-test/transcription_test.go README.md AGENTS.md
git commit -m "docs: document STT providers and breaking config change"
```

---

## Task 8: Manual smoke (user-driven)

- [ ] **OpenAI:** `STT_PROVIDER=openai` + existing key → `tran transcript` on a meeting dir

- [ ] **Groq:** `STT_PROVIDER=groq` + `GROQ_API_KEY` → short clip transcribes

- [ ] **whispercpp:** local model + `tran transcript` on one chunk

- [ ] **mlx (macOS):** `STT_PROVIDER=mlx`, `uvx` installed, first run downloads packages → transcript with segments

---

## Spec Coverage Checklist

| Spec requirement | Task |
|------------------|------|
| Mandatory `STT_PROVIDER` | 1, 2 |
| No default provider | 2 |
| `openai` provider | 3 |
| `groq` separate provider | 3 |
| `whispercpp` subprocess JSON | 4 |
| `mlx` via `uvx`, fail without uvx | 5 |
| `mlx` darwin-only stub | 5 |
| 25 MB prep cloud-only | 3 (in http provider) |
| Transcriber orchestration unchanged | 6 |
| Fail fast at startup | 6 |
| Breaking change docs | 7 |
| Unit tests fixtures | 3, 4 |
| Integration test skip | 7 |

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-02-stt-providers.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks  
2. **Inline Execution** — implement task-by-task in this session with checkpoints  

Which approach do you want?
