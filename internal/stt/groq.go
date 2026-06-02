package stt

import (
	"fmt"
	"net/http"

	"github.com/harnyk/tran/internal/config"
)

const groqTranscriptionsURL = "https://api.groq.com/openai/v1/audio/transcriptions"

func newGroqProvider(cfg *config.Config) (Provider, error) {
	if cfg.GroqAPIKey == "" {
		return nil, fmt.Errorf("GROQ_API_KEY is not set — get a key at https://console.groq.com/keys and add it to ~/.config/tran/config")
	}
	return &httpProvider{
		cfg:       cfg,
		url:       groqTranscriptionsURL,
		apiKey:    cfg.GroqAPIKey,
		model:     cfg.GroqModelSTT,
		ffmpegBin: cfg.FFmpegBin,
		apiName:   "Groq",
		client:    &http.Client{},
	}, nil
}
