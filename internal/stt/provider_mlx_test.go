//go:build !darwin

package stt

import (
	"strings"
	"testing"

	"github.com/harnyk/tran/internal/config"
)

func TestMLXOnLinux(t *testing.T) {
	_, err := NewProvider(&config.Config{STTProvider: "mlx"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "macOS") {
		t.Fatalf("got %v", err)
	}
}
