package stt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/harnyk/tran/internal/config"
)

const whisperCppRepoURL = "https://github.com/ggml-org/whisper.cpp"

type whisperCppProvider struct {
	bin       string
	modelPath string
	ffmpegBin string
}

func newWhisperCppProvider(cfg *config.Config) (Provider, error) {
	bin, err := exec.LookPath(cfg.WhisperCppBin)
	if err != nil {
		return nil, fmt.Errorf("%s not found in PATH — install with: brew install whisper-cpp\n# or build from %s",
			cfg.WhisperCppBin, whisperCppRepoURL)
	}

	if cfg.WhisperModelPath == "" {
		return nil, whisperCppModelError("WHISPER_MODEL_PATH is not set")
	}
	if _, err := os.Stat(cfg.WhisperModelPath); err != nil {
		return nil, whisperCppModelError(fmt.Sprintf("model not found at %q", cfg.WhisperModelPath))
	}

	return &whisperCppProvider{
		bin:       bin,
		modelPath: cfg.WhisperModelPath,
		ffmpegBin: cfg.FFmpegBin,
	}, nil
}

func whisperCppModelError(detail string) error {
	return fmt.Errorf("%s — download a ggml model, e.g.:\n  ./models/download-ggml-model.sh large-v3-turbo\nSee %s",
		detail, whisperCppRepoURL)
}

func (p *whisperCppProvider) Transcribe(ctx context.Context, audioPath, language string) (*Result, error) {
	tmpDir, err := os.MkdirTemp("", "tran-whispercpp-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	outBase := filepath.Join(tmpDir, "out")
	audio := audioPath

	if err := p.runWhisper(ctx, audio, language, outBase); err != nil {
		if !strings.EqualFold(filepath.Ext(audioPath), ".mp3") {
			return nil, err
		}
		wavPath := filepath.Join(tmpDir, "audio.wav")
		if convErr := p.convertToWAV(ctx, audioPath, wavPath); convErr != nil {
			return nil, err
		}
		audio = wavPath
		if err := p.runWhisper(ctx, audio, language, outBase); err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(outBase + ".json")
	if err != nil {
		return nil, fmt.Errorf("read whisper output: %w", err)
	}
	return parseVerboseJSON(data)
}

func (p *whisperCppProvider) runWhisper(ctx context.Context, audioPath, language, outBase string) error {
	langFlag := "auto"
	if language != "" {
		langFlag = language
	}

	cmd := exec.CommandContext(ctx, p.bin,
		"-m", p.modelPath,
		"-f", audioPath,
		"-l", langFlag,
		"-oj",
		"-of", outBase,
		"-np",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("whisper-cli: %w\n%s", err, stderr.String())
	}
	return nil
}

func (p *whisperCppProvider) convertToWAV(ctx context.Context, inPath, outPath string) error {
	cmd := exec.CommandContext(ctx, p.ffmpegBin,
		"-i", inPath,
		"-ar", "16000",
		"-ac", "1",
		"-y",
		outPath,
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("convert audio to WAV: %w\n%s", err, output.String())
	}
	return nil
}
