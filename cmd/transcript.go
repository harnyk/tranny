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

var transcriptCmd = &cobra.Command{
	Use:   "transcript",
	Short: "Transcribe MP3 chunks and write transcript.txt",
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

		fmt.Println("Transcribing...")
		if err := trans.TranscribeMeeting(ctx, m); err != nil {
			return err
		}

		info, _ := os.Stat(m.TranscriptPath())
		if info != nil {
			fmt.Printf("Saved: %s (%.1f KB)\n", m.TranscriptPath(), float64(info.Size())/1024)
		} else {
			fmt.Printf("Saved: %s\n", m.TranscriptPath())
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(transcriptCmd)
}
