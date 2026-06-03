package stt

import (
	"context"
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
