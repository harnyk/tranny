package stt

import (
	"bytes"
	"io"
	"mime/multipart"
	"os"
	"strings"
	"testing"

	"github.com/harnyk/tran/internal/config"
)

func TestWriteTranscriptionFields(t *testing.T) {
	t.Run("explicit language", func(t *testing.T) {
		fields := transcriptionFields(t, "whisper-1", "en")
		if got := fields["model"]; got != "whisper-1" {
			t.Fatalf("model field = %q, want %q", got, "whisper-1")
		}
		if got := fields["language"]; got != "en" {
			t.Fatalf("language field = %q, want %q", got, "en")
		}
		if got := fields["response_format"]; got != "verbose_json" {
			t.Fatalf("response_format field = %q, want %q", got, "verbose_json")
		}
		if got := fields["timestamp_granularities[]"]; got != "segment" {
			t.Fatalf("timestamp_granularities[] field = %q, want %q", got, "segment")
		}
	})

	t.Run("auto sends empty language", func(t *testing.T) {
		fields := transcriptionFields(t, "whisper-1", "")
		if got := fields["language"]; got != "" {
			t.Fatalf("language field = %q, want empty string", got)
		}
	})
}

func transcriptionFields(t *testing.T, model string, language string) map[string]string {
	t.Helper()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := writeTranscriptionFields(w, model, language); err != nil {
		t.Fatalf("writeTranscriptionFields() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	reader := multipart.NewReader(&body, w.Boundary())
	fields := make(map[string]string)

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reader.NextPart() error = %v", err)
		}

		value, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}
		fields[part.FormName()] = string(value)
	}

	return fields
}

func TestParseVerboseJSON(t *testing.T) {
	data, err := os.ReadFile("testdata/verbose_segments.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	res, err := parseVerboseJSON(data)
	if err != nil {
		t.Fatalf("parseVerboseJSON() error = %v", err)
	}
	if len(res.Segments) != 2 {
		t.Fatalf("len(Segments) = %d, want 2", len(res.Segments))
	}

	want := []Segment{
		{Start: 0.0, End: 1.5, Text: "hello"},
		{Start: 1.5, End: 2.0, Text: "world"},
	}
	for i, seg := range res.Segments {
		if seg != want[i] {
			t.Fatalf("Segments[%d] = %+v, want %+v", i, seg, want[i])
		}
	}
}

func TestNewOpenAIProviderMissingKey(t *testing.T) {
	_, err := NewProvider(&config.Config{STTProvider: "openai"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewGroqProviderMissingKey(t *testing.T) {
	_, err := NewProvider(&config.Config{STTProvider: "groq"})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "GROQ_API_KEY") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "console.groq.com") {
		t.Fatalf("unexpected error: %v", err)
	}
}
