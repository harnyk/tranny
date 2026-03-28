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

var compressCmd = &cobra.Command{
	Use:   "compress",
	Short: "Re-encode record.mkv to a smaller MP4 with only the mix audio track",
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

		fmt.Println("Compressing to MP4 (mix track only)...")
		out, err := comp.Compress(ctx, m)
		if err != nil {
			return err
		}

		info, _ := os.Stat(out)
		if info != nil {
			fmt.Printf("Created: %s (%.1f MB)\n", out, float64(info.Size())/1024/1024)
		} else {
			fmt.Printf("Created: %s\n", out)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(compressCmd)
}
