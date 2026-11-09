// Gobotto retains a model conversation for each addressed Chatto thread.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/bot"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/brainview"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/journal"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/mcpclient"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/memory"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/scheduler"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/subagent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
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
	memories, err := memory.NewConfigured(cfg.Memory)
	if err != nil {
		return err
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
	workers, err := subagent.Open(cfg.Subagents, func(model, id string, usage func(agent.Usage)) agent.Complete {
		return agent.OpenRouter(settings["OPENROUTER_API_KEY"], model, id, usage)
	})
	if err != nil {
		return err
	}
	defer workers.Close()
	external, err := mcpclient.Open(ctx, cfg.MCP, mcpclient.ChattoConnection{URL: settings["CHATTO_URL"], Token: settings["CHATTO_API_KEY"]})
	if err != nil {
		return err
	}
	defer external.Close()
	viewer, err := c.Viewer(ctx)
	if err != nil {
		return err
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
	schedules, err := scheduler.Open(filepath.Join(cfg.Memory.Directory, "schedules"), settings["CHATTO_URL"]+"\n"+viewer.ID)
	if err != nil {
		return err
	}
	defer schedules.Close()
	journals, err := journal.Open(filepath.Join(cfg.Memory.Directory, "conversations"), settings["CHATTO_URL"]+"\n"+viewer.ID)
	if err != nil {
		return err
	}
	defer journals.Close()
	return tui.RunWithBrains(ctx, brainview.New(cfg.Memory), func(ctx context.Context, emit tui.Emit) error {
		var titles agent.TitleGenerator
		titleModel := ""
		if cfg.Titles != nil {
			titleModel = cfg.Titles.Model
			titles = agent.OpenRouterTitles(settings["OPENROUTER_API_KEY"], cfg.Titles.Model, cfg.Titles.SystemPrompt)
		}
		return bot.Run(ctx, &chattoActions{Client: c, mcp: external}, viewer, func(id string, usage func(agent.Usage)) agent.Complete {
			options := agent.CompletionOptions{}
			if cfg.Conversation.MaxOutputTokens != nil {
				options.MaxOutputTokens = *cfg.Conversation.MaxOutputTokens
			}
			if r := cfg.Conversation.Reasoning; r != nil {
				options.Reasoning = &agent.ReasoningOptions{Enabled: r.Enabled, Effort: r.Effort, MaxTokens: r.MaxTokens, Exclude: r.Exclude}
			}
			return agent.OpenRouter(settings["OPENROUTER_API_KEY"], cfg.Conversation.Model, id, usage, options)
		}, emit, bot.Options{Journals: journals, Scheduler: schedules, Shutdown: tui.ShutdownRequested(ctx), Tools: external.Tools(), Subagents: workers,
			TurnTimeout: cfg.Conversation.TimeoutDuration, Limits: agent.Limits{MaxToolCalls: cfg.Conversation.MaxToolCalls, MaxModelSteps: cfg.Conversation.MaxModelSteps}, MaxDelegations: cfg.Conversation.MaxDelegations,
			Debounce: cfg.Conversation.DebounceDuration, Store: store, SystemPrompt: cfg.Conversation.SystemPrompt, OwnerUserIDs: cfg.OwnerUserIDs, Memory: memories, Titles: titles, Model: cfg.Conversation.Model, TitleModel: titleModel})
	})
}

// chattoActions keeps realtime and context intake on the SDK while using the
// same MCP operations as model-selected actions for all message delivery.
type chattoActions struct {
	*chatto.Client
	mcp *mcpclient.Manager
}

func (c *chattoActions) Reply(ctx context.Context, room, root, replyTo, body, key string) error {
	return c.mcp.Deliver(ctx, "post_message", map[string]string{"room_id": room, "thread_root_message_id": root, "in_reply_to_message_id": replyTo, "body": body, "idempotency_key": key})
}
func (c *chattoActions) React(ctx context.Context, room, message, emoji string) error {
	return c.mcp.Deliver(ctx, "add_reaction", map[string]string{"room_id": room, "message_id": message, "emoji": emoji})
}

// ObserveResumable forwards durable realtime checkpoints to MCP so writes and
// reads on another replica wait for the events already accepted by Gobotto.
func (c *chattoActions) ObserveResumable(ctx context.Context, viewer string, options chatto.RecoveryOptions, ready func(), handle func(context.Context, chatto.MessageEvent) error) error {
	c.mcp.SetChattoCursor(options.Cursor)
	options.Checkpoint = chattoCheckpoint(options.Checkpoint, c.mcp.SetChattoCursor)
	return c.Client.ObserveResumable(ctx, viewer, options, ready, handle)
}

// chattoCheckpoint publishes a consistency boundary only after durable inbox
// acceptance. Empty checkpoints clear an expired boundary during recovery.
func chattoCheckpoint(checkpoint func(string, *chatto.MessageEvent) error, publish func(string)) func(string, *chatto.MessageEvent) error {
	return func(cursor string, event *chatto.MessageEvent) error {
		if checkpoint != nil {
			if err := checkpoint(cursor, event); err != nil {
				return err
			}
		}
		publish(cursor)
		return nil
	}
}
