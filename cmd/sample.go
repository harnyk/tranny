package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/harnyk/tran/internal/meeting"
	"github.com/harnyk/tran/internal/sampler"
)

var sampleCmd = &cobra.Command{
	Use:   "sample",
	Short: "Extract speaker reference samples from mic and sys audio",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		m, err := meeting.Detect(cwd)
		if err != nil {
			return err
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		s := sampler.New(cfg)
		if err := s.ExtractSamples(ctx, m); err != nil {
			return err
		}

		fmt.Printf("Samples written to:\n  %s\n  %s\n", m.MicSamplePath(), m.SysSamplePath())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sampleCmd)
}
