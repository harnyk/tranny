package stt

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

	"github.com/harnyk/tran/internal/config"
)

const maxUploadBytes = 25 * 1024 * 1024

const openaiTranscriptionsURL = "https://api.openai.com/v1/audio/transcriptions"

type httpProvider struct {
	cfg       *config.Config
	url       string
	apiKey    string
	model     string
	ffmpegBin string
	apiName   string
	client    *http.Client
}

func newOpenAIProvider(cfg *config.Config) (Provider, error) {
	if cfg.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is not set — add it to ~/.config/tran/config")
	}
	return &httpProvider{
		cfg:       cfg,
		url:       openaiTranscriptionsURL,
		apiKey:    cfg.OpenAIAPIKey,
		model:     cfg.OpenAIModelSTT,
		ffmpegBin: cfg.FFmpegBin,
		apiName:   "OpenAI",
		client:    &http.Client{},
	}, nil
}

type verboseSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type verboseResponse struct {
	Text     string           `json:"text"`
	Segments []verboseSegment `json:"segments"`
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

func parseVerboseJSON(data []byte) (*Result, error) {
	var resp verboseResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	result := &Result{}
	for _, seg := range resp.Segments {
		result.Segments = append(result.Segments, Segment{
			Start: seg.Start,
			End:   seg.End,
			Text:  strings.TrimSpace(seg.Text),
		})
	}
	return result, nil
}

func prepareUploadAudio(ffmpegBin, path string) (outPath string, cleanup func(), err error) {
	noop := func() {}
	info, err := os.Stat(path)
	if err != nil {
		return "", noop, err
	}
	if info.Size() <= maxUploadBytes {
		return path, noop, nil
	}

	tmp, err := os.CreateTemp("", "tran-*.mp3")
	if err != nil {
		return "", noop, err
	}
	tmp.Close()

	cmd := exec.Command(ffmpegBin,
		"-i", path,
		"-ar", "16000",
		"-ac", "1",
		"-b:a", "64k",
		"-y",
		tmp.Name(),
	)
	var ffmpegOutput bytes.Buffer
	cmd.Stdout = &ffmpegOutput
	cmd.Stderr = &ffmpegOutput
	if err := cmd.Run(); err != nil {
		os.Remove(tmp.Name())
		return "", noop, fmt.Errorf("compress audio: %w\n%s", err, ffmpegOutput.String())
	}

	compressed, err := os.Stat(tmp.Name())
	if err != nil {
		os.Remove(tmp.Name())
		return "", noop, err
	}
	if compressed.Size() > maxUploadBytes {
		os.Remove(tmp.Name())
		return "", noop, fmt.Errorf("compressed file still exceeds 25MB limit")
	}

	return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
}

func (p *httpProvider) Transcribe(ctx context.Context, audioPath, language string) (*Result, error) {
	path, cleanup, err := prepareUploadAudio(p.ffmpegBin, audioPath)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := writeTranscriptionFields(w, p.model, language); err != nil {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s API error %d: %s", p.apiName, resp.StatusCode, string(respBytes))
	}

	return parseVerboseJSON(respBytes)
}
