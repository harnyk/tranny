package transcriber

import (
	"bytes"
	"io"
	"mime/multipart"
	"testing"
)

func TestNormalizeLanguage(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default english", input: "en", want: "en"},
		{name: "polish", input: "pl", want: "pl"},
		{name: "auto", input: "auto", want: ""},
		{name: "uppercase rejected", input: "EN", wantErr: true},
		{name: "three letters rejected", input: "eng", wantErr: true},
		{name: "region rejected", input: "en-US", wantErr: true},
		{name: "unknown code rejected", input: "zz", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeLanguage(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeLanguage(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeLanguage(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeLanguage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWriteTranscriptionFields(t *testing.T) {
	t.Run("explicit language", func(t *testing.T) {
		fields := transcriptionFields(t, "whisper-1", "en")
		if got := fields["model"]; got != "whisper-1" {
			t.Fatalf("model field = %q, want %q", got, "whisper-1")
		}
		if got := fields["language"]; got != "en" {
			t.Fatalf("language field = %q, want %q", got, "en")
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
