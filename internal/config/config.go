package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenAIAPIKey      string
	OpenAIModelSTT    string
	GroqAPIKey        string
	GroqModelSTT      string
	STTProvider       string
	FFmpegBin         string
	Display           string
	AudioTeeBin       string
	MicDeviceIndex    string
	WhisperCppBin     string
	WhisperModelPath  string
	UVXBin            string
	MLXWhisperModel   string
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
		OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
		OpenAIModelSTT:   envOrDefault("OPENAI_MODEL_STT", "whisper-1"),
		GroqAPIKey:       os.Getenv("GROQ_API_KEY"),
		GroqModelSTT:     envOrDefault("GROQ_MODEL_STT", "whisper-large-v3-turbo"),
		STTProvider:      os.Getenv("STT_PROVIDER"),
		FFmpegBin:        envOrDefault("FFMPEG_BIN", "ffmpeg"),
		Display:          envOrDefault("DISPLAY", ":0"),
		AudioTeeBin:      envOrDefault("AUDIOTEE_BIN", "audiotee"),
		MicDeviceIndex:   envOrDefault("AVFOUNDATION_MIC_INDEX", "0"),
		WhisperCppBin:    envOrDefault("WHISPER_CPP_BIN", "whisper-cli"),
		WhisperModelPath: os.Getenv("WHISPER_MODEL_PATH"),
		UVXBin:           envOrDefault("UVX_BIN", "uvx"),
		MLXWhisperModel:  envOrDefault("MLX_WHISPER_MODEL", "mlx-community/whisper-large-v3-turbo"),
	}
	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
