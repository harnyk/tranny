//go:build !darwin

package stt

import "github.com/harnyk/tran/internal/config"

func newMLXProvider(cfg *config.Config) (Provider, error) {
	return nil, mlxUnsupportedGOOS()
}
