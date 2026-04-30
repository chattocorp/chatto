//go:build bootstrap

package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/log"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

// devBootstrapFromConfig applies the [dev_bootstrap] section from chatto.toml
// to the running instance. Idempotent — entries that already exist (matched by
// login or by space name) are skipped. Errors on individual entries are logged
// but don't abort the rest, so the section behaves like "ensure this stuff
// exists" rather than a transactional batch.
//
// Only compiled into builds with the `bootstrap` tag; release binaries replace this
// with a no-op so the [dev_bootstrap] section in chatto.toml is parsed but
// ignored.
func devBootstrapFromConfig(ctx context.Context, c *core.ChattoCore, cfg config.DevBootstrapConfig) {
	if len(cfg.Users) == 0 && len(cfg.Spaces) == 0 {
		return
	}

	logger := log.WithPrefix("dev-bootstrap")
	logger.Info("Applying [dev_bootstrap] section", "users", len(cfg.Users), "spaces", len(cfg.Spaces))

	loginToUserID := map[string]string{}
	for _, u := range cfg.Users {
		userID, created := applyDevBootstrapUser(ctx, logger, c, u)
		if userID != "" {
			loginToUserID[u.Login] = userID
		}
		if created {
			logger.Info("Created user from [dev_bootstrap]", "login", u.Login, "user_id", userID)
		}
	}

	for _, s := range cfg.Spaces {
		applyDevBootstrapSpace(ctx, logger, c, s, loginToUserID)
	}
}

// applyDevBootstrapUser creates the user if missing, sets a verified email if
// the section has one, and assigns an instance role if specified. Returns the
// resolved user ID (whether existing or newly created) and whether we created it.
func applyDevBootstrapUser(ctx context.Context, logger *log.Logger, c *core.ChattoCore, u config.DevBootstrapUser) (string, bool) {
	if u.Login == "" {
		logger.Error("Skipping [dev_bootstrap] user with empty login")
		return "", false
	}

	if existing, err := c.GetUserByLogin(ctx, u.Login); err == nil && existing != nil {
		logger.Debug("[dev_bootstrap] user already exists; skipping create", "login", u.Login)
		// Still try to apply role + email below (idempotent).
		assignBootstrapRole(ctx, logger, c, existing.Id, u.InstanceRole, u.Login)
		ensureBootstrapEmail(ctx, logger, c, existing.Id, u.Email, u.Login)
		return existing.Id, false
	}

	displayName := u.DisplayName
	if displayName == "" {
		displayName = u.Login
	}

	user, err := c.CreateUser(ctx, "system", u.Login, displayName, u.Password)
	if err != nil {
		logger.Error("Failed to create [dev_bootstrap] user", "login", u.Login, "error", err)
		return "", false
	}

	ensureBootstrapEmail(ctx, logger, c, user.Id, u.Email, u.Login)
	assignBootstrapRole(ctx, logger, c, user.Id, u.InstanceRole, u.Login)

	return user.Id, true
}

func ensureBootstrapEmail(ctx context.Context, logger *log.Logger, c *core.ChattoCore, userID, email, login string) {
	if email == "" {
		return
	}
	if err := c.AddVerifiedEmailDirect(ctx, userID, email); err != nil {
		// ErrEmailAlreadyVerified is fine — the email is already attached.
		if !errors.Is(err, core.ErrEmailAlreadyVerified) {
			logger.Warn("Failed to add verified email for [dev_bootstrap] user", "login", login, "email", email, "error", err)
		}
	}
}

func assignBootstrapRole(ctx context.Context, logger *log.Logger, c *core.ChattoCore, userID, role, login string) {
	if role == "" {
		return
	}
	var roleName string
	switch role {
	case "owner":
		roleName = core.InstRoleOwner
	case "admin":
		roleName = core.InstRoleAdmin
	case "moderator":
		roleName = core.InstRoleModerator
	default:
		logger.Warn("Unknown instance_role in [dev_bootstrap]; ignoring", "login", login, "role", role)
		return
	}
	// SystemActorID bypasses hierarchy checks — bootstrap operates as the system.
	if err := c.AssignInstanceRole(ctx, core.SystemActorID, userID, roleName); err != nil {
		logger.Warn("Failed to assign instance role for [dev_bootstrap] user", "login", login, "role", role, "error", err)
	}
}

// applyDevBootstrapSpace creates the space if no existing space matches by name,
// then creates each requested room with auto_join=true. Owner is resolved by
// login from the users we just processed.
func applyDevBootstrapSpace(ctx context.Context, logger *log.Logger, c *core.ChattoCore, s config.DevBootstrapSpace, loginToUserID map[string]string) {
	if s.Name == "" {
		logger.Error("Skipping [dev_bootstrap] space with empty name")
		return
	}
	ownerID, ok := loginToUserID[s.OwnerLogin]
	if !ok {
		logger.Error("[dev_bootstrap] space references unknown owner_login; skipping",
			"space", s.Name, "owner_login", s.OwnerLogin)
		return
	}

	// Idempotency: skip if a space with this name already exists.
	if existing, err := findSpaceByName(ctx, c, s.Name); err == nil && existing != "" {
		logger.Debug("[dev_bootstrap] space already exists; skipping create", "name", s.Name)
		return
	}

	space, err := c.CreateSpace(ctx, ownerID, s.Name, s.Description)
	if err != nil {
		logger.Error("Failed to create [dev_bootstrap] space", "name", s.Name, "error", err)
		return
	}
	logger.Info("Created space from [dev_bootstrap]", "name", s.Name, "space_id", space.Id)

	for _, roomName := range s.Rooms {
		room, err := c.CreateRoom(ctx, ownerID, space.Id, roomName, "")
		if err != nil {
			logger.Warn("Failed to create [dev_bootstrap] room", "space", s.Name, "room", roomName, "error", err)
			continue
		}
		if _, err := c.SetRoomAutoJoin(ctx, ownerID, space.Id, room.Id, true); err != nil {
			logger.Warn("Failed to set auto_join on [dev_bootstrap] room", "space", s.Name, "room", roomName, "error", err)
		}
		if _, err := c.JoinRoom(ctx, ownerID, space.Id, ownerID, room.Id); err != nil {
			logger.Warn("Failed to join owner to [dev_bootstrap] room", "space", s.Name, "room", roomName, "error", err)
		}
	}
}

// findSpaceByName returns the ID of a live space matching name, or "" if not
// found. Used only by the dev bootstrap path; ListSpaces-style scan is fine
// for dev-sized data.
func findSpaceByName(ctx context.Context, c *core.ChattoCore, name string) (string, error) {
	spaces, err := c.ListSpaces(ctx)
	if err != nil {
		return "", fmt.Errorf("list spaces: %w", err)
	}
	for _, sp := range spaces {
		if sp.Name == name {
			return sp.Id, nil
		}
	}
	return "", nil
}
