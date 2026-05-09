//go:build bootstrap

package cmd

import (
	"context"
	"errors"

	"github.com/charmbracelet/log"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

// applyBootstrap applies the [bootstrap] section from chatto.toml to the
// running instance. Idempotent — entries that already exist (matched by login
// for users, by presence of the primary space record for the instance) are
// left alone. Errors on individual entries are logged but don't abort the
// rest, so the section behaves like "ensure this stuff exists" rather than
// a transactional batch.
//
// Only compiled into builds with the `bootstrap` tag; release binaries replace
// this with a no-op so the [bootstrap] section in chatto.toml is parsed but
// ignored.
func applyBootstrap(ctx context.Context, c *core.ChattoCore, cfg config.BootstrapConfig) {
	logger := log.WithPrefix("bootstrap")

	hasInstance := cfg.Instance != nil
	if len(cfg.Users) == 0 && !hasInstance {
		// Always log something so operators can confirm the bootstrap path ran.
		// At debug level so a config without a [bootstrap] section doesn't add
		// noise on every boot.
		logger.Debug("[bootstrap] section is empty; nothing to apply")
		return
	}

	logger.Info("Applying [bootstrap] section", "users", len(cfg.Users), "instance", hasInstance)

	loginToUserID := map[string]string{}
	ownerID := ""
	firstUserID := ""
	usersCreated, usersExisting := 0, 0
	for _, u := range cfg.Users {
		userID, created := applyBootstrapUser(ctx, logger, c, u)
		if userID == "" {
			continue
		}
		loginToUserID[u.Login] = userID
		if firstUserID == "" {
			firstUserID = userID
		}
		if ownerID == "" && u.InstanceRole == "owner" {
			ownerID = userID
		}
		if created {
			usersCreated++
			logger.Info("Created user from [bootstrap]", "login", u.Login, "user_id", userID)
		} else {
			usersExisting++
		}
	}

	if ownerID == "" {
		ownerID = firstUserID
	}

	instanceCreated := false
	if hasInstance {
		if ownerID == "" {
			logger.Error("[bootstrap] instance requires at least one user; skipping instance setup")
		} else {
			instanceCreated = applyBootstrapInstance(ctx, logger, c, *cfg.Instance, ownerID)
		}
	}

	// Mirror the regular signup flow: every bootstrap user joins the
	// deployment's server space. Bootstrap creates users via core.CreateUser
	// directly, which bypasses the auth signup path that normally calls this.
	// Without it, users other than the owner (e.g. alice/bob in the dev
	// config) would land in the instance with no space membership.
	for _, userID := range loginToUserID {
		c.JoinServer(ctx, userID)
	}

	logger.Info("[bootstrap] apply complete",
		"users_created", usersCreated,
		"users_existing", usersExisting,
		"instance_created", instanceCreated,
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

// applyBootstrapInstance seeds the instance's user-visible config (name)
// and ensures the deployment's primary room set exists. The underlying
// primary-space record is a transitional storage detail (per ADR-027 the
// data model still routes through a Space until PR(c) collapses the RBAC
// engines) — operators don't configure or see it directly. Returns true if
// a primary space was newly created, false otherwise (already-existing or
// skipped).
func applyBootstrapInstance(ctx context.Context, logger *log.Logger, c *core.ChattoCore, inst config.BootstrapInstance, ownerID string) bool {
	if inst.Name == "" {
		logger.Error("Skipping [bootstrap.instance] with empty name")
		return false
	}

	// Seed the runtime instance config (idempotent — only writes when the
	// name field is unset, so an admin-edited instance name isn't clobbered
	// on every dev restart).
	if cm := c.ConfigManager(); cm != nil {
		if _, err := cm.UpdateInstanceConfigFunc(ctx, func(current *configv1.InstanceConfig) (*configv1.InstanceConfig, error) {
			if current == nil {
				return &configv1.InstanceConfig{InstanceName: inst.Name}, nil
			}
			if current.InstanceName == "" {
				current.InstanceName = inst.Name
			}
			return current, nil
		}); err != nil {
			logger.Warn("Failed to seed instance config from [bootstrap.instance]", "error", err)
		}
	}

	// Idempotency: skip if a primary space already exists.
	if existing, err := c.FirstUserFacingSpaceID(ctx); err == nil && existing != "" {
		logger.Debug("[bootstrap] instance already has a primary space; skipping create")
		return false
	}

	space, err := c.CreateSpace(ctx, ownerID, inst.Name, "")
	if err != nil {
		logger.Error("Failed to create primary space from [bootstrap.instance]", "name", inst.Name, "error", err)
		return false
	}
	logger.Info("Created primary space from [bootstrap.instance]", "name", inst.Name, "space_id", space.Id)

	// When [bootstrap.instance] doesn't specify rooms, seed the same default
	// rooms a fresh deployment would get (announcements + general, both
	// auto-join).
	rooms := buildBootstrapRoomList(inst.Rooms)
	for _, r := range rooms {
		room, err := c.CreateRoom(ctx, ownerID, space.Id, r.Name, r.Description)
		if err != nil {
			logger.Warn("Failed to create [bootstrap] room", "room", r.Name, "error", err)
			continue
		}
		if _, err := c.SetRoomAutoJoin(ctx, ownerID, space.Id, room.Id, true); err != nil {
			logger.Warn("Failed to set auto_join on [bootstrap] room", "room", r.Name, "error", err)
		}
		if _, err := c.JoinRoom(ctx, ownerID, space.Id, ownerID, room.Id); err != nil {
			logger.Warn("Failed to join owner to [bootstrap] room", "room", r.Name, "error", err)
		}
	}

	// Issue #330 / ADR-027: in dev/E2E, bootstrap is the only way new users
	// land on the server, and they arrive as members (not owners). Grant
	// room.create to the everyone role so existing tests that create rooms as
	// the auto-joined user keep working without per-test permission setup.
	// Bootstrap only runs under the bootstrap build tag (dev/E2E), so this
	// never affects production.
	if err := c.GrantSpacePermission(ctx, ownerID, space.Id, core.SpaceRoleEveryone, core.PermRoomCreate); err != nil {
		logger.Warn("Failed to grant room.create to everyone on bootstrap instance", "error", err)
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

