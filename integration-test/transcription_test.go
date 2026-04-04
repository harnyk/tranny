//go:build integration

package integrationtest_test

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/harnyk/tran/internal/config"
	"github.com/harnyk/tran/internal/meeting"
	"github.com/harnyk/tran/internal/transcriber"
)

// werThreshold is the maximum acceptable Word Error Rate for transcription output.
// Whisper on clean TTS audio typically scores well under 5%; 15% gives ample headroom.
const werThreshold = 0.15

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

func TestDualChannelTranscription(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.OpenAIAPIKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	meetingPath, err := filepath.Abs("assets/meeting")
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}

	m := &meeting.MeetingDir{Path: meetingPath}

	// Remove generated transcript artifacts after the test run.
	// The transcript/ dir is gitignored so this keeps the working tree clean.
	t.Cleanup(func() {
		os.RemoveAll(m.TranscriptDir())
	})

	tr := transcriber.New(cfg)
	if err := tr.TranscribeMeetingDualChannel(context.Background(), m, "en"); err != nil {
		t.Fatalf("TranscribeMeetingDualChannel: %v", err)
	}

	lines := parseTranscript(t, m.TranscriptPath())
	if len(lines) == 0 {
		t.Fatal("transcript is empty")
	}

	// Per-speaker WER implicitly validates speaker assignment: if channels were
	// swapped, both WERs would spike because the wrong words would be under each label.
	checkWER(t, lines, "Us")
	checkWER(t, lines, "Them")
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

// checkWER concatenates all text for the given speaker from both reference and
// hypothesis, then asserts Word Error Rate is within threshold.
func checkWER(t *testing.T, lines []transcriptLine, speaker string) {
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
	if wer > werThreshold {
		t.Errorf("[%s] WER %.1f%% exceeds threshold %.1f%%", speaker, wer*100, werThreshold*100)
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

// editDistance computes Levenshtein distance between two string slices (word or token sequences).
func editDistance(a, b []string) int {
	m, n := len(a), len(b)
	// Use two rows to keep memory O(n).
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
