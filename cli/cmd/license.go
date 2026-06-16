package cmd

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

// LICENSE is a copy of the root LICENSE (go:embed can't reach outside the module).
//
//go:embed LICENSE
var licenseText string

// NOTICE is a copy of the root NOTICE (go:embed can't reach outside the module).
//
//go:embed NOTICE
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
