package transcriber

import (
	"bufio"
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
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/language"

	"github.com/harnyk/tran/internal/config"
	"github.com/harnyk/tran/internal/format"
	"github.com/harnyk/tran/internal/meeting"
)

const (
	whisperURL   = "https://api.openai.com/v1/audio/transcriptions"
	maxSizeBytes = 25 * 1024 * 1024 // 25 MB OpenAI limit
)

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
		return fmt.Errorf("OPENAI_API_KEY is not set — add it to ~/.config/tran/config")
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
		return fmt.Errorf("no MP3 files found — run 'tran mp3' first")
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

	base, err := language.ParseBase(lang)
	if err != nil {
		return "", fmt.Errorf("invalid --lang %q: use a valid ISO 639 language code (2 or 3 letters) like \"en\", \"eng\", or \"pol\", or \"auto\"", lang)
	}

	return base.String(), nil
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

// segmentToTempDir splits audioPath into 963s chunks in a new temp directory.
// The caller must defer os.RemoveAll(tempDir) to clean up.
func (t *Transcriber) segmentToTempDir(ctx context.Context, audioPath string) (chunks []string, tempDir string, err error) {
	tempDir, err = os.MkdirTemp("", "tran-seg-*")
	if err != nil {
		return nil, "", err
	}

	pattern := filepath.Join(tempDir, "chunk-%03d.mp3")
	cmd := exec.CommandContext(ctx, t.cfg.FFmpegBin,
		"-i", audioPath,
		"-f", "segment",
		"-segment_time", "963",
		"-segment_start_number", "1",
		"-y",
		pattern,
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		return nil, "", fmt.Errorf("segment audio: %w", err)
	}

	matches, err := filepath.Glob(filepath.Join(tempDir, "chunk-*.mp3"))
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, "", err
	}
	sort.Strings(matches)
	return matches, tempDir, nil
}

// TranscribeChannel transcribes a single audio file (e.g. source/mic.mp3) and writes
// an intermediate transcript to outputPath. Timestamps are absolute (chunk offsets applied).
// Lines are in the format: [M:SS.mmm] text
func (t *Transcriber) TranscribeChannel(ctx context.Context, audioPath, lang, outputPath string) error {
	chunks, tempDir, err := t.segmentToTempDir(ctx, audioPath)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if len(chunks) == 0 {
		return fmt.Errorf("no segments produced from %s", audioPath)
	}

	var sb strings.Builder
	var offset float64
	for i, chunk := range chunks {
		chunkNum := i + 1
		fmt.Fprintf(os.Stderr, "  chunk %03d/%03d: %s\n", chunkNum, len(chunks), filepath.Base(chunk))

		result, err := t.transcribeFile(ctx, chunk, lang)
		if err != nil {
			return fmt.Errorf("chunk %03d: %w", chunkNum, err)
		}

		for _, seg := range result.Segments {
			fmt.Fprintf(&sb, "[%s] %s\n", format.Timestamp(seg.Start+offset), strings.TrimSpace(seg.Text))
		}
		offset += result.Duration
	}

	return os.WriteFile(outputPath, []byte(sb.String()), 0644)
}

type channelSegment struct {
	seconds float64
	speaker string
	text    string
}

// parseTimestampSeconds converts a timestamp string produced by format.Timestamp back to seconds.
// Accepts M:SS.mmm or H:MM:SS.mmm (with or without milliseconds).
func parseTimestampSeconds(s string) (float64, error) {
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2: // M:SS.mmm
		mins, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
		secs, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}
		return mins*60 + secs, nil
	case 3: // H:MM:SS.mmm
		hours, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
		mins, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}
		secs, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, err
		}
		return hours*3600 + mins*60 + secs, nil
	default:
		return 0, fmt.Errorf("unrecognised timestamp %q", s)
	}
}

// parseChannelFile reads an intermediate transcript file and returns parsed segments tagged with speaker.
// Line format: [M:SS.mmm] text
func parseChannelFile(path, speaker string) ([]channelSegment, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var segs []channelSegment
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "[") {
			continue
		}
		end := strings.Index(line, "]")
		if end < 0 {
			continue
		}
		ts := line[1:end]
		text := strings.TrimSpace(line[end+1:])
		secs, err := parseTimestampSeconds(ts)
		if err != nil {
			continue
		}
		segs = append(segs, channelSegment{seconds: secs, speaker: speaker, text: text})
	}
	return segs, scanner.Err()
}

// MergeChannelTranscripts combines intermediate per-channel transcripts into a single
// transcript.txt with Us/Them speaker labels sorted by timestamp.
func (t *Transcriber) MergeChannelTranscripts(micPath, sysPath, outputPath, meetingName string) error {
	micSegs, err := parseChannelFile(micPath, "Us")
	if err != nil {
		return fmt.Errorf("read mic transcript: %w", err)
	}
	sysSegs, err := parseChannelFile(sysPath, "Them")
	if err != nil {
		return fmt.Errorf("read sys transcript: %w", err)
	}

	all := append(micSegs, sysSegs...)
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].seconds < all[j].seconds
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "---\nmeeting: %s\n---\n\n", meetingName)
	for _, seg := range all {
		fmt.Fprintf(&sb, "[%s] %s: %s\n", format.Timestamp(seg.seconds), seg.speaker, seg.text)
	}

	return os.WriteFile(outputPath, []byte(sb.String()), 0644)
}

// TranscribeMeetingDualChannel transcribes mic and sys channels separately, writes
// intermediate transcript_mic.txt and transcript_sys.txt, then merges into transcript.txt.
// If an intermediate file already exists it is reused (skips re-transcription).
func (t *Transcriber) TranscribeMeetingDualChannel(ctx context.Context, m *meeting.MeetingDir, lang string) error {
	if t.cfg.OpenAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is not set — add it to ~/.config/tran/config")
	}

	apiLanguage, err := NormalizeLanguage(lang)
	if err != nil {
		return err
	}

	if _, err := os.Stat(m.TranscriptMicPath()); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Transcribing mic channel (%s)...\n", m.MicMP3Path())
		if err := t.TranscribeChannel(ctx, m.MicMP3Path(), apiLanguage, m.TranscriptMicPath()); err != nil {
			return fmt.Errorf("mic channel: %w", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Mic transcript already exists, skipping re-transcription.\n")
	}

	if _, err := os.Stat(m.TranscriptSysPath()); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Transcribing sys channel (%s)...\n", m.SysMP3Path())
		if err := t.TranscribeChannel(ctx, m.SysMP3Path(), apiLanguage, m.TranscriptSysPath()); err != nil {
			return fmt.Errorf("sys channel: %w", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Sys transcript already exists, skipping re-transcription.\n")
	}

	fmt.Fprintf(os.Stderr, "Merging transcripts...\n")
	return t.MergeChannelTranscripts(m.TranscriptMicPath(), m.TranscriptSysPath(), m.TranscriptPath(), filepath.Base(m.Path))
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
	tmp, err := os.CreateTemp("", "tran-*.mp3")
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
