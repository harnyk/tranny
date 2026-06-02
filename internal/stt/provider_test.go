package stt

import (
	"strings"
	"testing"

	"github.com/harnyk/tran/internal/config"
)

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
