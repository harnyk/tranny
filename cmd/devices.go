package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/harnyk/tran/internal/devices"
)

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List audio input devices",
}

var devicesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List audio input devices available for recording",
	RunE: func(cmd *cobra.Command, args []string) error {
		list, err := devices.ListInputDevices(cfg.FFmpegBin)
		if err != nil {
			return err
		}

		switch runtime.GOOS {
		case "darwin":
			fmt.Println("AVFoundation audio input devices:")
			fmt.Println()
			fmt.Printf("  %-5s  %s\n", "INDEX", "NAME")
			for _, d := range list {
				suffix := ""
				if d.Index == cfg.MicDeviceIndex {
					suffix = "  (configured)"
				}
				fmt.Printf("  %-5s  %s%s\n", d.Index, d.Name, suffix)
			}
			fmt.Println()
			fmt.Printf("Configured: AVFOUNDATION_MIC_INDEX=%s\n", cfg.MicDeviceIndex)
			fmt.Println()
			fmt.Println("Set the index in ~/.config/tran/config to choose the microphone for tran rec.")
			fmt.Println("Avoid Bluetooth headset indices unless you want hands-free (low quality) audio.")
		case "linux":
			fmt.Println("PulseAudio capture sources:")
			fmt.Println()
			for _, d := range list {
				fmt.Printf("  %s\n", d.Name)
			}
			fmt.Println()
			fmt.Println("tran rec uses the PulseAudio default source for the microphone.")
		default:
			for _, d := range list {
				fmt.Printf("%s  %s\n", d.Index, d.Name)
			}
		}
		return nil
	},
}

func init() {
	devicesCmd.AddCommand(devicesListCmd)
	rootCmd.AddCommand(devicesCmd)
}
