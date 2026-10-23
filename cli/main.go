package main

import "github.com/chattocorp/chatto/cli/cmd"

func main() {
	cmd.SetVersion(Version)
	cmd.Execute()
}
