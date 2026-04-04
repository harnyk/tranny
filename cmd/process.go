package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/harnyk/tran/internal/converter"
	"github.com/harnyk/tran/internal/meeting"
	"github.com/harnyk/tran/internal/transcriber"
)

var processCmd = &cobra.Command{
	Use:   "process",
	Short: "Auto-detect and run missing pipeline steps (soundmix, transcript)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		lang, err := cmd.Flags().GetString("lang")
		if err != nil {
			return err
		}
		if _, err := transcriber.NormalizeLanguage(lang); err != nil {
			return err
		}

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

		dualChannel, _ := cmd.Flags().GetBool("dual-channel")
		didSomething := false

		if !dualChannel {
			mp3s, err := m.ListMixMP3s()
			if err != nil {
				return err
			}
			if len(mp3s) == 0 {
				fmt.Println("No mix MP3s found — mixing...")
				micDB, _ := cmd.Flags().GetFloat64("mic-volume")
				sysDB, _ := cmd.Flags().GetFloat64("sys-volume")
				result, err := conv.Convert(ctx, m, converter.Options{
					MicVolumeDB: micDB,
					SysVolumeDB: sysDB,
				})
				if err != nil {
					return err
				}
				fmt.Printf("Created %d segment(s)\n", len(result.Segments))
				didSomething = true
			}
		}

		if _, err := os.Stat(m.TranscriptPath()); os.IsNotExist(err) {
			fmt.Println("No transcript found — transcribing...")
			var transcriptErr error
			if dualChannel {
				transcriptErr = trans.TranscribeMeetingDualChannel(ctx, m, lang)
			} else {
				transcriptErr = trans.TranscribeMeeting(ctx, m, lang)
			}
			if transcriptErr != nil {
				return transcriptErr
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
	processCmd.Flags().StringP("lang", "l", "en", "transcription language: ISO 639 code (2 or 3 letters, e.g. en, eng, pol) or auto")
	processCmd.Flags().Float64("mic-volume", 0, "microphone volume adjustment in dB (e.g. 3 or -6)")
	processCmd.Flags().Float64("sys-volume", 0, "system audio volume adjustment in dB (e.g. 3 or -6)")
	processCmd.Flags().BoolP("dual-channel", "d", false, "Transcribe mic and sys channels separately with Us/Them speaker labels")
	rootCmd.AddCommand(processCmd)
}
