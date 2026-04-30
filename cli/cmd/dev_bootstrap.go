//go:build dev

package cmd

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"hmans.de/chatto/internal/core"
)

func init() {
	// Register dev bootstrap hook. Both paths run on startup; the env-var
	// bootstrap (single user via Bootstrap()) and the file bootstrap (richer
	// multi-user/multi-space via CHATTO_BOOTSTRAP_FILE) are independent and
	// can be combined.
	devStartupHook = func(ctx context.Context, c *core.ChattoCore) {
		devBootstrapFromEnv(ctx, c)
		devBootstrapFromFile(ctx, c)
	}
}

// devBootstrapFromEnv auto-bootstraps the instance from environment variables.
// Environment variables:
//   - CHATTO_BOOTSTRAP_LOGIN: Login name (required, or derived from email)
//   - CHATTO_BOOTSTRAP_EMAIL: Email address (required)
//   - CHATTO_BOOTSTRAP_PASSWORD: Password (required)
//   - CHATTO_BOOTSTRAP_DISPLAY_NAME: Display name (optional)
//   - CHATTO_BOOTSTRAP_SPACE_NAME: Space name to create (optional)
//   - CHATTO_BOOTSTRAP_SPACE_DESC: Space description (optional)
func devBootstrapFromEnv(ctx context.Context, chattoCore *core.ChattoCore) {
	logger := log.WithPrefix("dev-bootstrap")

	email := os.Getenv("CHATTO_BOOTSTRAP_EMAIL")
	if email == "" {
		logger.Debug("CHATTO_BOOTSTRAP_EMAIL not set, skipping auto-bootstrap")
		return
	}

	password := os.Getenv("CHATTO_BOOTSTRAP_PASSWORD")
	if password == "" {
		logger.Warn("CHATTO_BOOTSTRAP_PASSWORD not set, skipping auto-bootstrap")
		return
	}

	// Derive login from email if not explicitly set
	login := os.Getenv("CHATTO_BOOTSTRAP_LOGIN")
	if login == "" {
		login = strings.Split(email, "@")[0]
	}

	displayName := os.Getenv("CHATTO_BOOTSTRAP_DISPLAY_NAME")
	spaceName := os.Getenv("CHATTO_BOOTSTRAP_SPACE_NAME")
	spaceDesc := os.Getenv("CHATTO_BOOTSTRAP_SPACE_DESC")

	input := core.BootstrapInput{
		Login:            login,
		Email:            email,
		Password:         password,
		DisplayName:      displayName,
		SpaceName:        spaceName,
		SpaceDescription: spaceDesc,
	}

	result, err := chattoCore.Bootstrap(ctx, input)
	if err != nil {
		if errors.Is(err, core.ErrAlreadyBootstrapped) {
			logger.Debug("Instance already bootstrapped, skipping")
			return
		}
		logger.Error("Auto-bootstrap failed", "error", err)
		return
	}

	logger.Info("Auto-bootstrapped instance",
		"user_id", result.User.Id,
		"login", result.User.Login,
		"email", email,
	)
	if result.Space != nil {
		logger.Info("Created initial space",
			"space_id", result.Space.Id,
			"name", result.Space.Name,
		)
	}
}
