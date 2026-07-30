package main

import (
	"fmt"
	"io"
	"os"

	"hmans.de/authling"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help")) {
		fmt.Fprintln(stdout, "Usage: authling <command>")
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Commands:")
		fmt.Fprintln(stdout, "  version  Print the Authling version")
		return 0
	}

	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(stdout, "authling version %s\n", authling.Version)
		return 0
	}

	fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
	return 2
}
