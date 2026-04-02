package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/harnyk/tranny/internal/converter"
	"github.com/harnyk/tranny/internal/meeting"
)

var processCmd = &cobra.Command{
	Use:   "process",
	Short: "Auto-detect and run missing pipeline steps (soundmix, transcript)",
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

		mp3s, err := m.ListMixMP3s()
		if err != nil {
			return err
		}

		didSomething := false

		if len(mp3s) == 0 {
			fmt.Println("No mix MP3s found — mixing...")
			result, err := conv.Convert(ctx, m, converter.Options{})
			if err != nil {
				return err
			}
			mp3s = result.Segments
			fmt.Printf("Created %d segment(s)\n", len(mp3s))
			didSomething = true
		}

		if _, err := os.Stat(m.TranscriptPath()); os.IsNotExist(err) {
			fmt.Println("No transcript found — transcribing...")
			if err := trans.TranscribeMeeting(ctx, m); err != nil {
				return err
			}
			fmt.Printf("Saved: %s\n", m.TranscriptPath())
			didSomething = true
		}

		if !didSomething {
			fmt.Println("Meeting is already fully processed.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(processCmd)
}
