package transcriber

import (
	"testing"
)

func TestNormalizeLanguage(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "two letter english", input: "en", want: "en"},
		{name: "three letter polish", input: "pol", want: "pl"},
		{name: "auto", input: "auto", want: ""},
		{name: "uppercase canonicalized", input: "EN", want: "en"},
		{name: "three letter english canonicalized", input: "eng", want: "en"},
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
