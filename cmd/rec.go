package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		// Print media table before starting so devices are known.
		// Devices are detected inside Record(), so we print after it returns
		// or — for the table — we print it right after Record starts.
		// Because device detection happens inside Record(), we print the
		// directory now and the device table after recording completes.
		// Instead, print the table using info available from config + meeting dir.
		fmt.Println("Recording media:")
		fmt.Printf("  %-12s  Audio capture (microphone)\n", "mic.mp3")
		fmt.Printf("  %-12s  Audio capture (system monitor)\n", "sys.mp3")
		fmt.Printf("  %-12s  Mixed audio + screen capture %s\n", "record.mp4", cfg.Display)
		fmt.Println()
		fmt.Printf("Directory: %s\n", m.Path)
		fmt.Println()

		fmt.Println("Recording started")

		if err := rec.Record(ctx, m); err != nil {
			return fmt.Errorf("recording failed: %w", err)
		}

		d := rec.Duration()
		fmt.Printf("Recording stopped (duration %s)\n", formatDuration(d))
		fmt.Printf("\ncd %s\n", m.Path)
		return nil
	},
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func init() {
	rootCmd.AddCommand(recCmd)
}
