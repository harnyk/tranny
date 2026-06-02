//go:build darwin

package stt

import (
	"fmt"

	"github.com/harnyk/tran/internal/config"
)

func newMLXProvider(cfg *config.Config) (Provider, error) {
	return nil, fmt.Errorf("mlx provider: not implemented yet")
}
