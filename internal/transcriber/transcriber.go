package transcriber

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/harnyk/tranny/internal/config"
	"github.com/harnyk/tranny/internal/format"
	"github.com/harnyk/tranny/internal/meeting"
)

const (
	whisperURL   = "https://api.openai.com/v1/audio/transcriptions"
	maxSizeBytes = 25 * 1024 * 1024 // 25 MB OpenAI limit
)

var iso6391Languages = map[string]struct{}{
	"aa": {}, "ab": {}, "ae": {}, "af": {}, "ak": {}, "am": {}, "an": {}, "ar": {}, "as": {}, "av": {},
	"ay": {}, "az": {}, "ba": {}, "be": {}, "bg": {}, "bh": {}, "bi": {}, "bm": {}, "bn": {}, "bo": {},
	"br": {}, "bs": {}, "ca": {}, "ce": {}, "ch": {}, "co": {}, "cr": {}, "cs": {}, "cu": {}, "cv": {},
	"cy": {}, "da": {}, "de": {}, "dv": {}, "dz": {}, "ee": {}, "el": {}, "en": {}, "eo": {}, "es": {},
	"et": {}, "eu": {}, "fa": {}, "ff": {}, "fi": {}, "fj": {}, "fo": {}, "fr": {}, "fy": {}, "ga": {},
	"gd": {}, "gl": {}, "gn": {}, "gu": {}, "gv": {}, "ha": {}, "he": {}, "hi": {}, "ho": {}, "hr": {},
	"ht": {}, "hu": {}, "hy": {}, "hz": {}, "ia": {}, "id": {}, "ie": {}, "ig": {}, "ii": {}, "ik": {},
	"io": {}, "is": {}, "it": {}, "iu": {}, "ja": {}, "jv": {}, "ka": {}, "kg": {}, "ki": {}, "kj": {},
	"kk": {}, "kl": {}, "km": {}, "kn": {}, "ko": {}, "kr": {}, "ks": {}, "ku": {}, "kv": {}, "kw": {},
	"ky": {}, "la": {}, "lb": {}, "lg": {}, "li": {}, "ln": {}, "lo": {}, "lt": {}, "lu": {}, "lv": {},
	"mg": {}, "mh": {}, "mi": {}, "mk": {}, "ml": {}, "mn": {}, "mr": {}, "ms": {}, "mt": {}, "my": {},
	"na": {}, "nb": {}, "nd": {}, "ne": {}, "ng": {}, "nl": {}, "nn": {}, "no": {}, "nr": {}, "nv": {},
	"ny": {}, "oc": {}, "oj": {}, "om": {}, "or": {}, "os": {}, "pa": {}, "pi": {}, "pl": {}, "ps": {},
	"pt": {}, "qu": {}, "rm": {}, "rn": {}, "ro": {}, "ru": {}, "rw": {}, "sa": {}, "sc": {}, "sd": {},
	"se": {}, "sg": {}, "si": {}, "sk": {}, "sl": {}, "sm": {}, "sn": {}, "so": {}, "sq": {}, "sr": {},
	"ss": {}, "st": {}, "su": {}, "sv": {}, "sw": {}, "ta": {}, "te": {}, "tg": {}, "th": {}, "ti": {},
	"tk": {}, "tl": {}, "tn": {}, "to": {}, "tr": {}, "ts": {}, "tt": {}, "tw": {}, "ty": {}, "ug": {},
	"uk": {}, "ur": {}, "uz": {}, "ve": {}, "vi": {}, "vo": {}, "wa": {}, "wo": {}, "xh": {}, "yi": {},
	"yo": {}, "za": {}, "zh": {}, "zu": {},
}

type Transcriber struct {
	cfg    *config.Config
	client *http.Client
}

func New(cfg *config.Config) *Transcriber {
	return &Transcriber{cfg: cfg, client: &http.Client{}}
}

type whisperSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type whisperResponse struct {
	Text     string           `json:"text"`
	Duration float64          `json:"duration"`
	Segments []whisperSegment `json:"segments"`
}

// TranscribeMeeting transcribes all MP3 chunks in a meeting dir and writes transcript.txt.
func (t *Transcriber) TranscribeMeeting(ctx context.Context, m *meeting.MeetingDir, lang string) error {
	if t.cfg.OpenAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is not set — add it to ~/.config/tranny/config")
	}

	apiLanguage, err := NormalizeLanguage(lang)
	if err != nil {
		return err
	}

	mp3s, err := m.ListMixMP3s()
	if err != nil {
		return err
	}
	if len(mp3s) == 0 {
		return fmt.Errorf("no MP3 files found — run 'tranny mp3' first")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "---\nmeeting: %s\n---\n\n", filepath.Base(m.Path))
	for i, path := range mp3s {
		chunkNum := i + 1
		fmt.Fprintf(os.Stderr, "Transcribing chunk %03d/%03d: %s\n", chunkNum, len(mp3s), filepath.Base(path))

		result, err := t.transcribeFile(ctx, path, apiLanguage)
		if err != nil {
			return fmt.Errorf("chunk %03d: %w", chunkNum, err)
		}

		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&sb, "## Chunk %03d\n\n", chunkNum)
		for _, seg := range result.Segments {
			fmt.Fprintf(&sb, "[%s] %s\n", format.Timestamp(seg.Start), strings.TrimSpace(seg.Text))
		}
	}

	return os.WriteFile(m.TranscriptPath(), []byte(sb.String()), 0644)
}

func NormalizeLanguage(lang string) (string, error) {
	if lang == "auto" {
		return "", nil
	}
	if _, ok := iso6391Languages[lang]; ok {
		return lang, nil
	}
	return "", fmt.Errorf("invalid --lang %q: use a lowercase ISO 639-1 code like \"en\" or \"pl\", or \"auto\"", lang)
}

func writeTranscriptionFields(w *multipart.Writer, model string, language string) error {
	if err := w.WriteField("model", model); err != nil {
		return err
	}
	if err := w.WriteField("language", language); err != nil {
		return err
	}
	if err := w.WriteField("response_format", "verbose_json"); err != nil {
		return err
	}
	return w.WriteField("timestamp_granularities[]", "segment")
}

func (t *Transcriber) transcribeFile(ctx context.Context, audioPath string, language string) (*whisperResponse, error) {
	path, isTemp, err := t.prepareAudio(audioPath)
	if err != nil {
		return nil, err
	}
	if isTemp {
		defer os.Remove(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := writeTranscriptionFields(w, t.cfg.STTModel, language); err != nil {
		return nil, err
	}

	fw, err := w.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return nil, err
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, whisperURL, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.cfg.OpenAIAPIKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(respBytes))
	}

	var result whisperResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

// prepareAudio returns the path to use (may be a compressed temp file if >25MB).
func (t *Transcriber) prepareAudio(path string) (outPath string, isTemp bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, err
	}
	if info.Size() <= maxSizeBytes {
		return path, false, nil
	}

	// Compress to temp file: 64kbps mono 16kHz
	tmp, err := os.CreateTemp("", "tranny-*.mp3")
	if err != nil {
		return "", false, err
	}
	tmp.Close()

	cmd := exec.Command(t.cfg.FFmpegBin,
		"-i", path,
		"-ar", "16000",
		"-ac", "1",
		"-b:a", "64k",
		"-y",
		tmp.Name(),
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Remove(tmp.Name())
		return "", false, fmt.Errorf("compress audio: %w", err)
	}

	compressed, err := os.Stat(tmp.Name())
	if err != nil {
		os.Remove(tmp.Name())
		return "", false, err
	}
	if compressed.Size() > maxSizeBytes {
		os.Remove(tmp.Name())
		return "", false, fmt.Errorf("compressed file still exceeds 25MB limit")
	}

	return tmp.Name(), true, nil
}
