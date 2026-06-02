package stt

import (
	"fmt"

	"github.com/harnyk/tran/internal/config"
)

func newOpenAIProvider(cfg *config.Config) (Provider, error) {
	return nil, fmt.Errorf("openai provider: not implemented yet")
}
