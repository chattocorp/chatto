//go:build dev

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/pelletier/go-toml/v2"
	"hmans.de/chatto/internal/core"
)

// devBootstrapFile is the on-disk schema for `dev-bootstrap.toml`. Loaded only by
// dev builds via CHATTO_BOOTSTRAP_FILE. Plaintext passwords are fine here — the
// file is dev-only and not loaded by release binaries.
type devBootstrapFile struct {
	Users  []devBootstrapUser  `toml:"users"`
	Spaces []devBootstrapSpace `toml:"spaces"`
}

type devBootstrapUser struct {
	Login        string `toml:"login"`         // required
	DisplayName  string `toml:"display_name"`  // defaults to Login
	Email        string `toml:"email"`         // optional; auto-verified if set
	Password     string `toml:"password"`      // optional (no-password = OAuth-only)
	InstanceRole string `toml:"instance_role"` // optional: owner | admin | moderator
}

type devBootstrapSpace struct {
	Name        string   `toml:"name"`        // required
	Description string   `toml:"description"` // optional
	OwnerLogin  string   `toml:"owner_login"` // required — must match a Users entry's login
	Rooms       []string `toml:"rooms"`       // optional; auto-join rooms created in the space
}

// devBootstrapFromFile reads CHATTO_BOOTSTRAP_FILE and applies its contents to
// the running instance. Idempotent — entries that already exist (matched by
// login or by space name) are skipped. Errors on individual entries are logged
// but don't abort the rest, so the seed file behaves like "ensure this stuff
// exists" rather than a transactional batch.
func devBootstrapFromFile(ctx context.Context, chattoCore *core.ChattoCore) {
	logger := log.WithPrefix("dev-bootstrap")

	path := os.Getenv("CHATTO_BOOTSTRAP_FILE")
	if path == "" {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Warn("CHATTO_BOOTSTRAP_FILE points at a missing file; skipping", "path", path)
			return
		}
		logger.Error("Failed to read bootstrap file", "path", path, "error", err)
		return
	}

	var spec devBootstrapFile
	if err := toml.Unmarshal(data, &spec); err != nil {
		logger.Error("Failed to parse bootstrap file", "path", path, "error", err)
		return
	}

	logger.Info("Applying bootstrap file", "path", path, "users", len(spec.Users), "spaces", len(spec.Spaces))

	loginToUserID := map[string]string{}
	for _, u := range spec.Users {
		userID, created := applyDevBootstrapUser(ctx, logger, chattoCore, u)
		if userID != "" {
			loginToUserID[u.Login] = userID
		}
		if created {
			logger.Info("Created user from bootstrap file", "login", u.Login, "user_id", userID)
		}
	}

	for _, s := range spec.Spaces {
		applyDevBootstrapSpace(ctx, logger, chattoCore, s, loginToUserID)
	}
}

// applyDevBootstrapUser creates the user if missing, sets a verified email if
// the file has one, and assigns an instance role if specified. Returns the
// resolved user ID (whether existing or newly created) and whether we created it.
func applyDevBootstrapUser(ctx context.Context, logger *log.Logger, c *core.ChattoCore, u devBootstrapUser) (string, bool) {
	if u.Login == "" {
		logger.Error("Skipping bootstrap user with empty login")
		return "", false
	}

	if existing, err := c.GetUserByLogin(ctx, u.Login); err == nil && existing != nil {
		logger.Debug("Bootstrap user already exists; skipping create", "login", u.Login)
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
		logger.Error("Failed to create bootstrap user", "login", u.Login, "error", err)
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
			logger.Warn("Failed to add verified email for bootstrap user", "login", login, "email", email, "error", err)
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
		logger.Warn("Unknown instance_role in bootstrap file; ignoring", "login", login, "role", role)
		return
	}
	// SystemActorID bypasses hierarchy checks — bootstrap operates as the system.
	if err := c.AssignInstanceRole(ctx, core.SystemActorID, userID, roleName); err != nil {
		logger.Warn("Failed to assign instance role for bootstrap user", "login", login, "role", role, "error", err)
	}
}

// applyDevBootstrapSpace creates the space if no existing space matches by name,
// then creates each requested room with auto_join=true. Owner is resolved by
// login from the users we just processed.
func applyDevBootstrapSpace(ctx context.Context, logger *log.Logger, c *core.ChattoCore, s devBootstrapSpace, loginToUserID map[string]string) {
	if s.Name == "" {
		logger.Error("Skipping bootstrap space with empty name")
		return
	}
	ownerID, ok := loginToUserID[s.OwnerLogin]
	if !ok {
		logger.Error("Bootstrap space references unknown owner_login; skipping",
			"space", s.Name, "owner_login", s.OwnerLogin)
		return
	}

	// Idempotency: skip if a space with this name already exists.
	if existing, err := findSpaceByName(ctx, c, s.Name); err == nil && existing != "" {
		logger.Debug("Bootstrap space already exists; skipping create", "name", s.Name)
		return
	}

	space, err := c.CreateSpace(ctx, ownerID, s.Name, s.Description)
	if err != nil {
		logger.Error("Failed to create bootstrap space", "name", s.Name, "error", err)
		return
	}
	logger.Info("Created space from bootstrap file", "name", s.Name, "space_id", space.Id)

	for _, roomName := range s.Rooms {
		room, err := c.CreateRoom(ctx, ownerID, space.Id, roomName, "")
		if err != nil {
			logger.Warn("Failed to create bootstrap room", "space", s.Name, "room", roomName, "error", err)
			continue
		}
		if _, err := c.SetRoomAutoJoin(ctx, ownerID, space.Id, room.Id, true); err != nil {
			logger.Warn("Failed to set auto_join on bootstrap room", "space", s.Name, "room", roomName, "error", err)
		}
		if _, err := c.JoinRoom(ctx, ownerID, space.Id, ownerID, room.Id); err != nil {
			logger.Warn("Failed to join owner to bootstrap room", "space", s.Name, "room", roomName, "error", err)
		}
	}
}

// findSpaceByName returns the ID of a live space matching name, or "" if not
// found. Used only by the dev bootstrap path; CountSpaces-style scan is fine
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
