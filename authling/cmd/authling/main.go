package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"hmans.de/authling"
	"hmans.de/authling/internal/app"
	"hmans.de/authling/internal/config"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(runContext(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runContext(context.Background(), args, stdout, stderr)
}

func runContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help")) {
		fmt.Fprintln(stdout, "Usage: authling <command>")
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Commands:")
		fmt.Fprintln(stdout, "  run      Run the Authling service")
		fmt.Fprintln(stdout, "  version  Print the Authling version")
		return 0
	}

	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(stdout, "authling version %s\n", authling.Version)
		return 0
	}

	if args[0] == "run" {
		flags := flag.NewFlagSet("authling run", flag.ContinueOnError)
		flags.SetOutput(stderr)
		configPath := flags.String("config", "", "path to configuration file (default: authling.toml)")
		flags.StringVar(configPath, "c", "", "path to configuration file (default: authling.toml)")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if flags.NArg() != 0 {
			fmt.Fprintln(stderr, "authling run does not accept positional arguments")
			return 2
		}
		cfg, err := config.Read(*configPath)
		if err != nil {
			fmt.Fprintf(stderr, "load configuration: %v\n", err)
			return 1
		}
		logger := slog.New(slog.NewTextHandler(stderr, nil))
		if err := app.Serve(ctx, cfg, logger); err != nil {
			fmt.Fprintf(stderr, "run Authling: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
	return 2
}
