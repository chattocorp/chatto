package main

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/charmbracelet/colorprofile"
)

func main() {
	server := flag.String("server", os.Getenv("CHATTO_URL"), "Chatto server origin (or CHATTO_URL)")
	images := flag.String("images", "auto", "Attachment previews: auto, kitty, or off")
	flag.Parse()
	if *images != "auto" && *images != "kitty" && *images != "off" {
		fmt.Fprintln(os.Stderr, "Chatty: --images must be auto, kitty, or off")
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	e, err := newEngine(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Chatty:", err)
		os.Exit(1)
	}
	defer e.close()
	if *server == "" {
		*server = e.state.Session.Origin
	}
	if e.state.Session.Origin != "" && strings.TrimRight(*server, "/") != e.state.Session.Origin {
		fmt.Fprintln(os.Stderr, "Chatty: sign out of the saved server before selecting another server")
		return
	}
	m := newModel(e, *server)
	m.images.enabled = *images == "kitty" || (*images == "auto" && automaticImages())
	opts := []tea.ProgramOption{tea.WithContext(ctx)}
	if m.images.enabled {
		// Placeholder colors encode IDs; palette reduction would corrupt them.
		opts = append(opts, tea.WithColorProfile(colorprofile.TrueColor))
	}
	if _, err := tea.NewProgram(m, opts...).Run(); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "Chatty: terminal session ended unexpectedly")
	}
	// Also release terminal image storage after an external interrupt or error.
	m.images.clear()
	if m.images.raw.Len() > 0 {
		fmt.Fprint(os.Stdout, m.images.raw.String())
	}
	cancel()
}
