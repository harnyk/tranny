package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/harnyk/tranny/internal/recorder"
)

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List available recording profiles",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, name := range []string{"default", "telegram", "lowres"} {
			p := recorder.Profiles[name]
			fmt.Printf("%-10s  %s\n", p.Name, p.Description)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(profilesCmd)
}
