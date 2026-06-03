//go:build integration

package integrationtest_test

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode"

	"github.com/harnyk/tran/internal/config"
	"github.com/harnyk/tran/internal/meeting"
	"github.com/harnyk/tran/internal/transcriber"
)

// werThreshold is the maximum acceptable Word Error Rate for cloud STT on clean TTS audio.
const werThreshold = 0.15

// mlxWerThreshold is slightly higher: local mlx-whisper occasionally repeats a segment.
const mlxWerThreshold = 0.25

// referenceDialog mirrors the dialog hardcoded in prepare/main.go.
// "Us" = mic channel (male), "Them" = sys channel (female).
var referenceDialog = []struct {
	speaker string
	text    string
}{
	{"Us", "Hi Sarah, thanks for joining the call. Should we get started?"},
	{"Them", "Absolutely, happy to be here. I've been looking forward to this sync."},
	{"Us", "Great. So the main topic today is the project status. Backend is mostly wrapped up — we just need to finalize the API endpoints."},
	{"Them", "That's good to hear. On my side, the UI is about eighty percent complete. I'm currently finishing the dashboard component and the data table."},
	{"Us", "Do you foresee any blockers before the end of the week?"},
	{"Them", "Nothing critical so far. I might need about thirty minutes of your time to clarify the response format for the analytics endpoint."},
	{"Us", "Sure, I can do that right after this call. I'll also send you the updated schema in Slack."},
	{"Them", "Perfect. That should be everything I need. Do you think we're on track for the Friday deadline?"},
	{"Us", "I think so. As long as the integration tests pass, we should be good to ship."},
	{"Them", "Agreed. Let's sync again on Thursday just to confirm. I'll send a calendar invite."},
}

func TestDualChannelTranscription_OpenAI(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.STTProvider != "openai" || cfg.OpenAIAPIKey == "" {
		t.Skip("requires STT_PROVIDER=openai and OPENAI_API_KEY")
	}
	runDualChannelTranscription(t, cfg)
}

func TestDualChannelTranscription_MLX(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("mlx provider is macOS only")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.STTProvider != "mlx" {
		t.Skip("requires STT_PROVIDER=mlx")
	}
	if _, err := exec.LookPath(cfg.UVXBin); err != nil {
		t.Skipf("requires %q in PATH (install uv: https://docs.astral.sh/uv/)", cfg.UVXBin)
	}
	runDualChannelTranscription(t, cfg)
}

func runDualChannelTranscription(t *testing.T, cfg *config.Config) {
	t.Helper()

	meetingPath, err := filepath.Abs("assets/meeting")
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}

	m := &meeting.MeetingDir{Path: meetingPath}

	os.RemoveAll(m.TranscriptDir())
	t.Cleanup(func() {
		os.RemoveAll(m.TranscriptDir())
	})

	tr, err := transcriber.New(cfg)
	if err != nil {
		t.Fatalf("transcriber.New: %v", err)
	}
	if err := tr.TranscribeMeetingDualChannel(context.Background(), m, "en"); err != nil {
		t.Fatalf("TranscribeMeetingDualChannel: %v", err)
	}

	lines := parseTranscript(t, m.TranscriptPath())
	if len(lines) == 0 {
		t.Fatal("transcript is empty")
	}

	checkWER(t, lines, "Us", werThresholdFor(cfg))
	checkWER(t, lines, "Them", werThresholdFor(cfg))
}

func werThresholdFor(cfg *config.Config) float64 {
	if cfg.STTProvider == "mlx" {
		return mlxWerThreshold
	}
	return werThreshold
}

// transcriptLine is one parsed line from transcript.txt.
type transcriptLine struct {
	speaker string
	text    string
}

// Matches timestamps like [0:05.123] or [1:05:30.456]
var transcriptLineRe = regexp.MustCompile(`^\[\d+:\d{2}(?::\d{2})?\.\d{3}\] (Us|Them): (.+)$`)

func parseTranscript(t *testing.T, path string) []transcriptLine {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	var lines []transcriptLine
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		m := transcriptLineRe.FindStringSubmatch(scanner.Text())
		if m != nil {
			lines = append(lines, transcriptLine{speaker: m[1], text: m[2]})
		}
	}
	return lines
}

func checkWER(t *testing.T, lines []transcriptLine, speaker string, threshold float64) {
	t.Helper()

	var refParts, hypParts []string
	for _, r := range referenceDialog {
		if r.speaker == speaker {
			refParts = append(refParts, r.text)
		}
	}
	for _, l := range lines {
		if l.speaker == speaker {
			hypParts = append(hypParts, l.text)
		}
	}

	if len(hypParts) == 0 {
		t.Errorf("[%s] no lines found in transcript", speaker)
		return
	}

	refWords := strings.Fields(normalize(strings.Join(refParts, " ")))
	hypWords := strings.Fields(normalize(strings.Join(hypParts, " ")))

	dist := editDistance(refWords, hypWords)
	wer := float64(dist) / float64(len(refWords))
	t.Logf("[%s] WER: %.1f%% (%d edits, ref=%d words, hyp=%d words)", speaker, wer*100, dist, len(refWords), len(hypWords))
	if wer > threshold {
		t.Errorf("[%s] WER %.1f%% exceeds threshold %.1f%%", speaker, wer*100, threshold*100)
	}
}

func normalize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsSpace(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func editDistance(a, b []string) int {
	m, n := len(a), len(b)
	prev := make([]int, n+1)
	curr := make([]int, n+1)
	for j := 0; j <= n; j++ {
		prev[j] = j
	}
	for i := 1; i <= m; i++ {
		curr[0] = i
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1]
			} else {
				curr[j] = 1 + min(prev[j], min(curr[j-1], prev[j-1]))
			}
		}
		prev, curr = curr, prev
	}
	return prev[n]
}
