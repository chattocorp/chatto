// Gobotto retains a model conversation for each addressed Chatto thread.
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/bot"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/mcpclient"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/memory"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/treememory"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	"github.com/chatto-chat/chatto/experimental/gobotto/memorytree"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "gobotto:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	path := os.Getenv("GOBOTTO_CONFIG")
	if path == "" {
		path = "gobotto.toml"
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	var memories *memory.Manager
	var tree *treememory.Manager
	if cfg.Memory.Backend == "means" {
		memories, err = memory.New(cfg.Memory.Collections, cfg.Memory.Directory)
		if err != nil {
			return err
		}
	}
	if cfg.Memory.Backend == "tree" {
		db, openErr := memorytree.Open(cfg.Memory.Database, "Be helpful, curious, and concise. Learn useful facts and organize them for later recall.")
		if openErr != nil {
			return errors.New("cannot open tree memory database")
		}
		defer db.Close()
		tree = &treememory.Manager{Store: db}
	}
	settings := map[string]string{}
	for _, name := range []string{"CHATTO_URL", "CHATTO_API_KEY", "OPENROUTER_API_KEY"} {
		settings[name] = strings.TrimSpace(os.Getenv(name))
		if settings[name] == "" {
			return fmt.Errorf("set %s", name)
		}
	}
	c, err := chatto.New(settings["CHATTO_URL"], settings["CHATTO_API_KEY"])
	if err != nil {
		return err
	}
	return tui.Run(ctx, func(ctx context.Context, emit tui.Emit) error {
		external, err := mcpclient.Open(ctx, cfg.MCP)
		if err != nil {
			return err
		}
		defer external.Close()
		viewer, err := c.Viewer(ctx)
		if err != nil {
			return err
		}
		if tree != nil {
			binding := sha256.Sum256([]byte(strings.TrimRight(settings["CHATTO_URL"], "/") + "\x00" + viewer.ID))
			if err := tree.Store.BindIdentity(ctx, fmt.Sprintf("%x", binding)); err != nil {
				return errors.New("tree memory identity check failed; use a separate database for each bot/server")
			}
			tree.Changed = func(s memorytree.Snapshot) { emit(tui.Event{Kind: tui.MemoryChanged, Memory: &s}) }
			s, err := tree.Store.Snapshot(ctx)
			if err != nil {
				return errors.New("cannot load tree memory")
			}
			tree.Changed(s)
		}
		dir := os.Getenv("GOBOTTO_SESSION_DIR")
		if dir == "" {
			dir = ".gobotto/sessions"
		}
		store, err := bot.OpenStore(dir, settings["CHATTO_URL"], viewer.ID, cfg.Conversation.Model)
		if err != nil {
			return err
		}
		defer store.Close()
		var titles agent.TitleGenerator
		titleModel := ""
		if cfg.Titles != nil {
			titleModel = cfg.Titles.Model
			titles = agent.OpenRouterTitles(settings["OPENROUTER_API_KEY"], cfg.Titles.Model, cfg.Titles.SystemPrompt)
		}
		return bot.Run(ctx, c, viewer, func(id string, usage func(agent.Usage)) agent.Complete {
			options := agent.CompletionOptions{}
			if cfg.Conversation.MaxOutputTokens != nil {
				options.MaxOutputTokens = *cfg.Conversation.MaxOutputTokens
			}
			if r := cfg.Conversation.Reasoning; r != nil {
				options.Reasoning = &agent.ReasoningOptions{Enabled: r.Enabled, Effort: r.Effort, MaxTokens: r.MaxTokens, Exclude: r.Exclude}
			}
			return agent.OpenRouter(settings["OPENROUTER_API_KEY"], cfg.Conversation.Model, id, usage, options)
		}, emit, bot.Options{Tree: tree, Shutdown: tui.ShutdownRequested(ctx), Tools: external.Tools(), Debounce: cfg.Conversation.DebounceDuration, Store: store, SystemPrompt: cfg.Conversation.SystemPrompt, OwnerUserIDs: cfg.OwnerUserIDs, Memory: memories, Titles: titles, Model: cfg.Conversation.Model, TitleModel: titleModel})
	})
}
