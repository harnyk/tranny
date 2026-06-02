//go:build darwin

package stt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/harnyk/tran/internal/config"
)

var mlxFirstRun sync.Once

type mlxProvider struct {
	uvx       string
	model     string
	ffmpegBin string
}

func newMLXProvider(cfg *config.Config) (Provider, error) {
	if _, err := exec.LookPath(cfg.UVXBin); err != nil {
		return nil, fmt.Errorf(
			"STT_PROVIDER=mlx requires uvx (https://docs.astral.sh/uv/)\n" +
				"Install: curl -LsSf https://astral.sh/uv/install.sh | sh",
		)
	}
	return &mlxProvider{
		uvx:       cfg.UVXBin,
		model:     cfg.MLXWhisperModel,
		ffmpegBin: cfg.FFmpegBin,
	}, nil
}

func (p *mlxProvider) Transcribe(ctx context.Context, audioPath, language string) (*Result, error) {
	mlxFirstRun.Do(func() {
		fmt.Println("Using mlx-whisper via uvx (first run may download packages)")
	})

	tmpDir, err := os.MkdirTemp("", "tran-mlx-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	args := []string{
		"--from", "mlx-whisper", "mlx_whisper",
		"--model", p.model,
		"--output-format", "json",
		"--output-dir", tmpDir,
		"--output-name", "out",
		"--verbose", "false",
	}
	if language != "" {
		args = append(args, "--language", language)
	}
	args = append(args, audioPath)

	cmd := exec.CommandContext(ctx, p.uvx, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("mlx_whisper: %w\n%s", err, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "out.json"))
	if err != nil {
		return nil, fmt.Errorf("read mlx output: %w", err)
	}
	return parseVerboseJSON(data)
}
