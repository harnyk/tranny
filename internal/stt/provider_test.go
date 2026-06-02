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

func TestNewWhisperCppMissingModel(t *testing.T) {
	_, err := NewProvider(&config.Config{
		STTProvider:    "whispercpp",
		WhisperCppBin:  "/usr/bin/true",
		WhisperModelPath: "",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "WHISPER_MODEL_PATH") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "ggml-org/whisper.cpp") {
		t.Fatalf("unexpected error: %v", err)
	}
}
