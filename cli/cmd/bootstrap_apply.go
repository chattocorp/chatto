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

// applyBootstrap applies the [bootstrap] section from chatto.toml to the
// running instance. Idempotent — entries that already exist (matched by login
// or by space name) are skipped. Errors on individual entries are logged but
// don't abort the rest, so the section behaves like "ensure this stuff exists"
// rather than a transactional batch.
//
// Only compiled into builds with the `bootstrap` tag; release binaries replace
// this with a no-op so the [bootstrap] section in chatto.toml is parsed but
// ignored.
func applyBootstrap(ctx context.Context, c *core.ChattoCore, cfg config.BootstrapConfig) {
	logger := log.WithPrefix("bootstrap")

	if len(cfg.Users) == 0 && len(cfg.Spaces) == 0 {
		// Always log something so operators can confirm the bootstrap path ran.
		// At debug level so a config without a [bootstrap] section doesn't add
		// noise on every boot.
		logger.Debug("[bootstrap] section is empty; nothing to apply")
		return
	}

	logger.Info("Applying [bootstrap] section", "users", len(cfg.Users), "spaces", len(cfg.Spaces))

	loginToUserID := map[string]string{}
	usersCreated, usersExisting := 0, 0
	for _, u := range cfg.Users {
		userID, created := applyBootstrapUser(ctx, logger, c, u)
		if userID == "" {
			continue
		}
		loginToUserID[u.Login] = userID
		if created {
			usersCreated++
			logger.Info("Created user from [bootstrap]", "login", u.Login, "user_id", userID)
		} else {
			usersExisting++
		}
	}

	spacesCreated, spacesExisting := 0, 0
	for _, s := range cfg.Spaces {
		if applyBootstrapSpace(ctx, logger, c, s, loginToUserID) {
			spacesCreated++
		} else {
			spacesExisting++
		}
	}

	logger.Info("[bootstrap] apply complete",
		"users_created", usersCreated,
		"users_existing", usersExisting,
		"spaces_created", spacesCreated,
		"spaces_existing", spacesExisting,
	)
}

// applyBootstrapUser creates the user if missing, sets a verified email if the
// section has one, and assigns an instance role if specified. Returns the
// resolved user ID (whether existing or newly created) and whether we created it.
func applyBootstrapUser(ctx context.Context, logger *log.Logger, c *core.ChattoCore, u config.BootstrapUser) (string, bool) {
	if u.Login == "" {
		logger.Error("Skipping [bootstrap] user with empty login")
		return "", false
	}

	if existing, err := c.GetUserByLogin(ctx, u.Login); err == nil && existing != nil {
		logger.Debug("[bootstrap] user already exists; skipping create", "login", u.Login)
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
		logger.Error("Failed to create [bootstrap] user", "login", u.Login, "error", err)
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
			logger.Warn("Failed to add verified email for [bootstrap] user", "login", login, "email", email, "error", err)
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
		logger.Warn("Unknown instance_role in [bootstrap]; ignoring", "login", login, "role", role)
		return
	}
	// SystemActorID bypasses hierarchy checks — bootstrap operates as the system.
	if err := c.AssignInstanceRole(ctx, core.SystemActorID, userID, roleName); err != nil {
		logger.Warn("Failed to assign instance role for [bootstrap] user", "login", login, "role", role, "error", err)
	}
}

// applyBootstrapSpace creates the space if no existing space matches by name,
// then creates each requested room with auto_join=true. Owner is resolved by
// login from the users we just processed. Returns true if a new space was
// created, false otherwise (already-existing or skipped).
func applyBootstrapSpace(ctx context.Context, logger *log.Logger, c *core.ChattoCore, s config.BootstrapSpace, loginToUserID map[string]string) bool {
	if s.Name == "" {
		logger.Error("Skipping [bootstrap] space with empty name")
		return false
	}
	ownerID, ok := loginToUserID[s.OwnerLogin]
	if !ok {
		logger.Error("[bootstrap] space references unknown owner_login; skipping",
			"space", s.Name, "owner_login", s.OwnerLogin)
		return false
	}

	// Idempotency: skip if a space with this name already exists.
	if existing, err := findSpaceByName(ctx, c, s.Name); err == nil && existing != "" {
		logger.Debug("[bootstrap] space already exists; skipping create", "name", s.Name)
		return false
	}

	space, err := c.CreateSpace(ctx, ownerID, s.Name, s.Description)
	if err != nil {
		logger.Error("Failed to create [bootstrap] space", "name", s.Name, "error", err)
		return false
	}
	logger.Info("Created space from [bootstrap]", "name", s.Name, "space_id", space.Id)

	// When the [bootstrap] section doesn't specify rooms, seed the same default
	// rooms a fresh space would get from the GraphQL createSpace mutation
	// (announcements + general, both auto-join). This keeps dev/E2E spaces
	// behaving like user-created spaces without each operator having to
	// repeat the room list in chatto.toml.
	rooms := buildBootstrapRoomList(s.Rooms)
	for _, r := range rooms {
		room, err := c.CreateRoom(ctx, ownerID, space.Id, r.Name, r.Description)
		if err != nil {
			logger.Warn("Failed to create [bootstrap] room", "space", s.Name, "room", r.Name, "error", err)
			continue
		}
		if _, err := c.SetRoomAutoJoin(ctx, ownerID, space.Id, room.Id, true); err != nil {
			logger.Warn("Failed to set auto_join on [bootstrap] room", "space", s.Name, "room", r.Name, "error", err)
		}
		if _, err := c.JoinRoom(ctx, ownerID, space.Id, ownerID, room.Id); err != nil {
			logger.Warn("Failed to join owner to [bootstrap] room", "space", s.Name, "room", r.Name, "error", err)
		}
	}
	return true
}

// buildBootstrapRoomList returns the rooms to create in a bootstrap space. If
// the operator specified an explicit list, those are used as-is (with empty
// descriptions). Otherwise we fall back to the same default rooms a fresh
// user-created space gets.
func buildBootstrapRoomList(specified []string) []core.DefaultAutoJoinRoom {
	if len(specified) == 0 {
		return core.DefaultAutoJoinRooms
	}
	out := make([]core.DefaultAutoJoinRoom, 0, len(specified))
	for _, name := range specified {
		out = append(out, core.DefaultAutoJoinRoom{Name: name})
	}
	return out
}

// findSpaceByName returns the ID of a live space matching name, or "" if not
// found. Used only by the bootstrap path; ListSpaces-style scan is fine for
// dev-sized data.
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
