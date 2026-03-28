package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/harnyk/tranny/internal/meeting"
	"github.com/harnyk/tranny/internal/recorder"
)

var recProfileName string

var recCmd = &cobra.Command{
	Use:   "rec [meeting name]",
	Short: "Record screen and audio for a new meeting",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		profile, err := recorder.GetProfile(recProfileName)
		if err != nil {
			return err
		}

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
		if err := m.WriteProfile(profile.Name); err != nil {
			return fmt.Errorf("write profile: %w", err)
		}

		outputPath := m.RecordPath(profile.Ext)

		fmt.Printf("Recording to: %s\n", outputPath)
		fmt.Printf("Profile: %s — %s\n", profile.Name, profile.Description)
		fmt.Println("Press Ctrl+C to stop recording.")

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if err := rec.Record(ctx, outputPath, profile); err != nil {
			return fmt.Errorf("recording failed: %w", err)
		}

		fmt.Println("\nRecording saved.")
		fmt.Printf("\ncd %s\n", m.Path)
		return nil
	},
}

func init() {
	recCmd.Flags().StringVar(&recProfileName, "profile", "default",
		"Recording profile: default, telegram, lowres")
	rootCmd.AddCommand(recCmd)
}
