package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/harnyk/tran/internal/config"
)

const (
	ttsModel          = "tts-1"
	ttsURL            = "https://api.openai.com/v1/audio/speech"
	pauseBetweenLines = 1.2 // seconds of silence between lines
	micVoice          = "onyx" // male
	sysVoice          = "nova" // female
)

type dialogLine struct {
	speaker string // "mic" or "sys"
	text    string
}

var dialog = []dialogLine{
	{"mic", "Hi Sarah, thanks for joining the call. Should we get started?"},
	{"sys", "Absolutely, happy to be here. I've been looking forward to this sync."},
	{"mic", "Great. So the main topic today is the project status. Backend is mostly wrapped up — we just need to finalize the API endpoints."},
	{"sys", "That's good to hear. On my side, the UI is about eighty percent complete. I'm currently finishing the dashboard component and the data table."},
	{"mic", "Do you foresee any blockers before the end of the week?"},
	{"sys", "Nothing critical so far. I might need about thirty minutes of your time to clarify the response format for the analytics endpoint."},
	{"mic", "Sure, I can do that right after this call. I'll also send you the updated schema in Slack."},
	{"sys", "Perfect. That should be everything I need. Do you think we're on track for the Friday deadline?"},
	{"mic", "I think so. As long as the integration tests pass, we should be good to ship."},
	{"sys", "Agreed. Let's sync again on Thursday just to confirm. I'll send a calendar invite."},
}

type clipInfo struct {
	speaker   string
	tmpFile   string
	startMs   int64
	durationS float64
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fatalf("load config: %v", err)
	}
	if cfg.OpenAIAPIKey == "" {
		fatalf("OPENAI_API_KEY is not set — add it to ~/.config/tran/config")
	}

	outDir := filepath.Join("integration-test", "assets", "meeting", "source")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fatalf("mkdir %s: %v", outDir, err)
	}

	tmpDir, err := os.MkdirTemp("", "tran-prepare-*")
	if err != nil {
		fatalf("mktemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Phase 1: generate TTS clips
	clips := make([]clipInfo, len(dialog))
	for i, line := range dialog {
		voice := micVoice
		if line.speaker == "sys" {
			voice = sysVoice
		}
		tmpFile := filepath.Join(tmpDir, fmt.Sprintf("clip-%02d.mp3", i))
		fmt.Printf("[%d/%d] TTS %s: %q\n", i+1, len(dialog), line.speaker, line.text)
		if err := generateTTS(cfg.OpenAIAPIKey, line.text, voice, tmpFile); err != nil {
			fatalf("TTS line %d: %v", i, err)
		}
		dur, err := getAudioDuration(tmpFile)
		if err != nil {
			fatalf("duration line %d: %v", i, err)
		}
		clips[i] = clipInfo{
			speaker:   line.speaker,
			tmpFile:   tmpFile,
			durationS: dur,
		}
	}

	// Phase 2: compute start times
	var cursor float64
	for i := range clips {
		clips[i].startMs = int64(cursor * 1000)
		cursor += clips[i].durationS + pauseBetweenLines
	}
	totalDuration := cursor - pauseBetweenLines // no trailing pause

	// Phase 3: assemble tracks
	micOut := filepath.Join(outDir, "mic.mp3")
	sysOut := filepath.Join(outDir, "sys.mp3")

	fmt.Println("Building mic.mp3 ...")
	if err := buildTrack(cfg.FFmpegBin, clips, "mic", totalDuration, micOut); err != nil {
		fatalf("build mic track: %v", err)
	}
	fmt.Println("Building sys.mp3 ...")
	if err := buildTrack(cfg.FFmpegBin, clips, "sys", totalDuration, sysOut); err != nil {
		fatalf("build sys track: %v", err)
	}

	recordOut := filepath.Join(outDir, "record.mp4")
	fmt.Println("Building record.mp4 ...")
	if err := buildRecording(cfg.FFmpegBin, micOut, sysOut, totalDuration, recordOut); err != nil {
		fatalf("build record: %v", err)
	}

	fmt.Printf("Done. Output:\n  %s\n  %s\n  %s\n", micOut, sysOut, recordOut)
}

// buildRecording mixes mic and sys audio over a black video of the same duration.
func buildRecording(ffmpegBin, micFile, sysFile string, totalDuration float64, outFile string) error {
	args := []string{
		"-y",
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=1280x720:r=25:d=%.3f", totalDuration),
		"-i", micFile,
		"-i", sysFile,
		"-filter_complex", "[1][2]amix=inputs=2:normalize=0[audio]",
		"-map", "0:v",
		"-map", "[audio]",
		"-c:v", "libx264", "-preset", "ultrafast", "-tune", "stillimage",
		"-c:a", "aac",
		"-t", fmt.Sprintf("%.3f", totalDuration),
		outFile,
	}
	cmd := exec.Command(ffmpegBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func generateTTS(apiKey, text, voice, outFile string) error {
	reqBody, err := json.Marshal(map[string]string{
		"model":           ttsModel,
		"input":           text,
		"voice":           voice,
		"response_format": "mp3",
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, ttsURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("TTS API returned %d: %s", resp.StatusCode, body)
	}

	f, err := os.Create(outFile)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func getAudioDuration(file string) (float64, error) {
	out, err := exec.Command("ffprobe",
		"-i", file,
		"-show_entries", "format=duration",
		"-v", "quiet",
		"-of", "csv=p=0",
	).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

// buildTrack assembles all clips for the given speaker into a single mp3,
// placing each clip at its computed start offset (with silence elsewhere).
func buildTrack(ffmpegBin string, clips []clipInfo, speaker string, totalDuration float64, outFile string) error {
	var speakerClips []clipInfo
	for _, c := range clips {
		if c.speaker == speaker {
			speakerClips = append(speakerClips, c)
		}
	}
	if len(speakerClips) == 0 {
		return fmt.Errorf("no clips for speaker %s", speaker)
	}

	args := []string{"-y"}
	for _, c := range speakerClips {
		args = append(args, "-i", c.tmpFile)
	}

	// Build filter_complex
	var filterParts []string
	var mixInputs []string
	for i, c := range speakerClips {
		delayMs := c.startMs
		tag := fmt.Sprintf("[a%d]", i)
		filterParts = append(filterParts,
			fmt.Sprintf("[%d]adelay=%d|%d%s", i, delayMs, delayMs, tag))
		mixInputs = append(mixInputs, tag)
	}

	var filterComplex string
	if len(speakerClips) == 1 {
		filterComplex = filterParts[0] + ";[a0]apad[out]"
	} else {
		mixChain := strings.Join(mixInputs, "") +
			fmt.Sprintf("amix=inputs=%d:normalize=0,apad[out]", len(speakerClips))
		filterComplex = strings.Join(filterParts, ";") + ";" + mixChain
	}

	args = append(args,
		"-filter_complex", filterComplex,
		"-map", "[out]",
		"-t", fmt.Sprintf("%.3f", totalDuration),
		outFile,
	)

	cmd := exec.Command(ffmpegBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
