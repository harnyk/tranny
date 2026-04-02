package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/harnyk/tran/internal/meeting"
)

var recCmd = &cobra.Command{
	Use:   "rec [meeting name]",
	Short: "Record screen and audio for a new meeting",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		m, err := meeting.NewMeetingDir(cwd, name)
		if err != nil {
			return err
		}

		fmt.Printf("Recording to: %s\n", m.Path)
		fmt.Println("Press Ctrl+C to stop recording.")

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if err := rec.Record(ctx, m); err != nil {
			return fmt.Errorf("recording failed: %w", err)
		}

		fmt.Println("\nRecording saved.")
		fmt.Printf("\ncd %s\n", m.Path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(recCmd)
}
