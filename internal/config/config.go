package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenAIAPIKey string
	STTModel     string
	FFmpegBin    string
	Display      string
}

func Load() (*Config, error) {
	// Load ~/.config/tran/config if it exists (don't error if missing)
	home, err := os.UserHomeDir()
	if err == nil {
		cfgFile := filepath.Join(home, ".config", "tran", "config")
		// godotenv.Overload sets env vars; we use Read to get values without polluting env
		vals, _ := godotenv.Read(cfgFile)
		for k, v := range vals {
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}

	cfg := &Config{
		OpenAIAPIKey: os.Getenv("OPENAI_API_KEY"),
		STTModel:     envOrDefault("OPENAI_MODEL_STT", "whisper-1"),
		FFmpegBin:    envOrDefault("FFMPEG_BIN", "ffmpeg"),
		Display:      envOrDefault("DISPLAY", ":0"),
	}
	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
