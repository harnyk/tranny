package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/harnyk/tranny/internal/config"
	"github.com/harnyk/tranny/internal/converter"
	"github.com/harnyk/tranny/internal/recorder"
	"github.com/harnyk/tranny/internal/transcriber"
)

var (
	cfg   *config.Config
	rec   *recorder.Recorder
	conv  *converter.Converter
	trans *transcriber.Transcriber
)

var rootCmd = &cobra.Command{
	Use:   "tranny",
	Short: "Meeting recorder and transcript manager",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initServices)
}

func initServices() {
	var err error
	cfg, err = config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}
	rec = recorder.New(cfg)
	conv = converter.New(cfg)
	trans = transcriber.New(cfg)
}
