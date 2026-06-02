package transcriber

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"

	"github.com/harnyk/tran/internal/config"
	"github.com/harnyk/tran/internal/converter"
	"github.com/harnyk/tran/internal/format"
	"github.com/harnyk/tran/internal/meeting"
	"github.com/harnyk/tran/internal/stt"
)

type Transcriber struct {
	cfg      *config.Config
	provider stt.Provider
}

func New(cfg *config.Config) (*Transcriber, error) {
	provider, err := stt.NewProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &Transcriber{cfg: cfg, provider: provider}, nil
}

// TranscribeMeeting transcribes all MP3 chunks in a meeting dir and writes transcript.txt.
func (t *Transcriber) TranscribeMeeting(ctx context.Context, m *meeting.MeetingDir, lang string) error {
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

	if err := os.MkdirAll(m.TranscriptDir(), 0755); err != nil {
		return err
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

func (t *Transcriber) transcribeFile(ctx context.Context, audioPath string, language string) (*stt.Result, error) {
	result, err := t.provider.Transcribe(ctx, audioPath, language)
	if err != nil {
		return nil, err
	}
	if len(result.Segments) == 0 {
		return nil, fmt.Errorf("no segments in transcription response")
	}
	return result, nil
}

// channelTranscript is the YAML schema for intermediate per-channel transcript files.
type channelTranscript struct {
	Meeting string        `yaml:"meeting"`
	Channel string        `yaml:"channel"`
	Chunks  []chunkRecord `yaml:"chunks"`
}

type chunkRecord struct {
	Index    int             `yaml:"index"`
	Offset   float64         `yaml:"offset"`
	Segments []segmentRecord `yaml:"segments"`
}

type segmentRecord struct {
	Start float64 `yaml:"start"`
	End   float64 `yaml:"end"`
	Text  string  `yaml:"text"`
}

// segmentToTempDir splits audioPath into SegmentTime-second chunks in a new temp directory.
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
		"-segment_time", fmt.Sprintf("%d", converter.SegmentTime),
		"-segment_start_number", "1",
		"-y",
		pattern,
	)
	var ffmpegOutput bytes.Buffer
	cmd.Stdout = &ffmpegOutput
	cmd.Stderr = &ffmpegOutput
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		return nil, "", fmt.Errorf("segment audio: %w\n%s", err, ffmpegOutput.String())
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
// a YAML intermediate transcript to outputPath.
// Offset is computed as float64(chunkIndex) * SegmentTime — no dependency on Whisper's
// reported duration, which can be unreliable for silent or very short chunks.
func (t *Transcriber) TranscribeChannel(ctx context.Context, audioPath, lang, outputPath, channel, meetingName string) error {
	chunks, tempDir, err := t.segmentToTempDir(ctx, audioPath)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if len(chunks) == 0 {
		return fmt.Errorf("no segments produced from %s", audioPath)
	}

	ct := channelTranscript{
		Meeting: meetingName,
		Channel: channel,
	}

	for i, chunk := range chunks {
		chunkNum := i + 1
		offset := float64(i) * converter.SegmentTime
		fmt.Printf("  %-4s chunk %d/%d\n", channel, chunkNum, len(chunks))

		result, err := t.transcribeFile(ctx, chunk, lang)
		if err != nil {
			return fmt.Errorf("chunk %03d: %w", chunkNum, err)
		}

		rec := chunkRecord{Index: chunkNum, Offset: offset}
		for _, seg := range result.Segments {
			rec.Segments = append(rec.Segments, segmentRecord{
				Start: seg.Start,
				End:   seg.End,
				Text:  strings.TrimSpace(seg.Text),
			})
		}
		ct.Chunks = append(ct.Chunks, rec)
	}

	data, err := yaml.Marshal(&ct)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}

type mergeSegment struct {
	absSeconds float64
	speaker    string
	text       string
}

// readChannelTranscript reads a YAML intermediate file and returns merge-ready segments.
func readChannelTranscript(path, speaker string) ([]mergeSegment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ct channelTranscript
	if err := yaml.Unmarshal(data, &ct); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var segs []mergeSegment
	for _, chunk := range ct.Chunks {
		for _, seg := range chunk.Segments {
			segs = append(segs, mergeSegment{
				absSeconds: chunk.Offset + seg.Start,
				speaker:    speaker,
				text:       seg.Text,
			})
		}
	}
	return segs, nil
}

// MergeChannelTranscripts combines intermediate per-channel YAML transcripts into a single
// transcript.txt with Us/Them speaker labels sorted by timestamp.
func (t *Transcriber) MergeChannelTranscripts(micPath, sysPath, outputPath, meetingName string) error {
	micSegs, err := readChannelTranscript(micPath, "Us")
	if err != nil {
		return fmt.Errorf("read mic transcript: %w", err)
	}
	sysSegs, err := readChannelTranscript(sysPath, "Them")
	if err != nil {
		return fmt.Errorf("read sys transcript: %w", err)
	}

	all := append(micSegs, sysSegs...)
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].absSeconds < all[j].absSeconds
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "---\nmeeting: %s\n---\n\n", meetingName)
	for _, seg := range all {
		fmt.Fprintf(&sb, "[%s] %s: %s\n", format.Timestamp(seg.absSeconds), seg.speaker, seg.text)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(sb.String()), 0644)
}

// TranscribeMeetingDualChannel transcribes mic and sys channels separately, writes
// intermediate transcript_mic.txt and transcript_sys.txt, then merges into transcript.txt.
// If an intermediate file already exists it is reused (skips re-transcription).
func (t *Transcriber) TranscribeMeetingDualChannel(ctx context.Context, m *meeting.MeetingDir, lang string) error {
	apiLanguage, err := NormalizeLanguage(lang)
	if err != nil {
		return err
	}

	meetingName := filepath.Base(m.Path)

	if _, err := os.Stat(m.TranscriptMicPath()); os.IsNotExist(err) {
		if err := t.TranscribeChannel(ctx, m.MicMP3Path(), apiLanguage, m.TranscriptMicPath(), "mic", meetingName); err != nil {
			return fmt.Errorf("mic channel: %w", err)
		}
	}

	if _, err := os.Stat(m.TranscriptSysPath()); os.IsNotExist(err) {
		if err := t.TranscribeChannel(ctx, m.SysMP3Path(), apiLanguage, m.TranscriptSysPath(), "sys", meetingName); err != nil {
			return fmt.Errorf("sys channel: %w", err)
		}
	}

	fmt.Println("  merging channels")
	return t.MergeChannelTranscripts(m.TranscriptMicPath(), m.TranscriptSysPath(), m.TranscriptPath(), meetingName)
}
