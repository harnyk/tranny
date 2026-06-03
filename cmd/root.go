package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/harnyk/tran/internal/config"
	"github.com/harnyk/tran/internal/converter"
	"github.com/harnyk/tran/internal/recorder"
	"github.com/harnyk/tran/internal/transcriber"
)

var (
	cfg   *config.Config
	rec   *recorder.Recorder
	conv  *converter.Converter
	trans *transcriber.Transcriber
)

var rootCmd = &cobra.Command{
	Use:   "tran",
	Short: "Meeting recorder and transcript manager",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	var err error
	cfg, err = config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}
	rec = recorder.New(cfg)
	conv = converter.New(cfg)
}

func ensureTranscriber() error {
	if trans != nil {
		return nil
	}
	var err error
	trans, err = transcriber.New(cfg)
	if err != nil {
		return err
	}
	return nil
}
