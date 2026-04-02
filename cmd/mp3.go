package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/harnyk/tranny/internal/meeting"
)

var mp3Cmd = &cobra.Command{
	Use:   "mp3",
	Short: "Normalize, mix, and segment source audio into MP3 chunks",
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

		fmt.Println("Converting to MP3 segments...")
		result, err := conv.Convert(ctx, m)
		if err != nil {
			return err
		}

		fmt.Printf("Created %d segment(s):\n", len(result.Segments))
		for _, s := range result.Segments {
			info, _ := os.Stat(s)
			if info != nil {
				fmt.Printf("  %s (%.1f MB)\n", s, float64(info.Size())/1024/1024)
			} else {
				fmt.Printf("  %s\n", s)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(mp3Cmd)
}
