package cmd

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

// LICENSE is generated from the root LICENSE before builds.
//
//go:embed embedded/LICENSE
var licenseText string

// NOTICE is generated from the root NOTICE before builds.
//
//go:embed embedded/NOTICE
var noticeText string

var licenseCmd = &cobra.Command{
	Use:   "license",
	Short: "Print license information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(licenseText)
		fmt.Print("\n\n")
		fmt.Print(noticeText)
	},
}

func init() {
	rootCmd.AddCommand(licenseCmd)
}
