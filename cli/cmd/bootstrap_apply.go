//go:build bootstrap

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

// applyBootstrap applies the [bootstrap] section from chatto.toml to an empty
// server. It is first-boot provisioning, not a recurring reconciliation loop:
// once application data exists, the section is ignored on later starts.
// Errors on individual entries are logged but don't abort the rest.
//
// Only compiled into builds with the `bootstrap` tag; release binaries replace
// this with a no-op so the [bootstrap] section in chatto.toml is parsed but
// ignored.
func applyBootstrap(ctx context.Context, c *core.ChattoCore, cfg config.BootstrapConfig) {
	logger := log.WithPrefix("bootstrap")

	serverConfig := cfg.ServerOrDefault()
	hasServer := serverConfig != nil
	if len(cfg.Users) == 0 && len(cfg.Bots) == 0 && !hasServer {
		// Always log something so operators can confirm the bootstrap path ran.
		// At debug level so a config without a [bootstrap] section doesn't add
		// noise on every boot.
		logger.Debug("[bootstrap] section is empty; nothing to apply")
		return
	}

	logger.Info("Applying [bootstrap] section", "users", len(cfg.Users), "bots", len(cfg.Bots), "server", hasServer)

	if empty, err := serverDataEmptyForBootstrap(ctx, c); err != nil {
		logger.Warn("Could not determine whether server is empty; skipping [bootstrap]", "error", err)
		return
	} else if !empty {
		logger.Debug("Server already has data; skipping [bootstrap]")
		return
	}

	ownerID := ""
	firstUserID := ""
	var bootstrapUserIDs []string
	usersCreated, usersExisting := 0, 0
	for _, u := range cfg.Users {
		userID, created := applyBootstrapUser(ctx, logger, c, u)
		if userID == "" {
			continue
		}
		bootstrapUserIDs = append(bootstrapUserIDs, userID)
		if firstUserID == "" {
			firstUserID = userID
		}
		if ownerID == "" && u.RoleOrDefault() == "owner" {
			ownerID = userID
		}
		if created {
			usersCreated++
			logger.Info("Created user from [bootstrap]", "user_id", userID)
		} else {
			usersExisting++
		}
	}

	if ownerID == "" {
		ownerID = firstUserID
	}

	serverCreated := false
	if hasServer {
		if ownerID == "" {
			logger.Error("[bootstrap] server requires at least one user; skipping server setup")
		} else {
			serverCreated = applyBootstrapServer(ctx, logger, c, *serverConfig, ownerID, bootstrapUserIDs)
		}
	}

	botsCreated := 0
	for _, bot := range cfg.Bots {
		if applyBootstrapBot(ctx, logger, c, bot) {
			botsCreated++
		}
	}

	logger.Info("[bootstrap] apply complete",
		"users_created", usersCreated,
		"users_existing", usersExisting,
		"bots_created", botsCreated,
		"server_created", serverCreated,
	)
}

// applyBootstrapBot creates one development bot, applies its owner-delegated
// server permissions, joins configured rooms, and writes its show-once API
// key. The key is never logged or stored in EVT.
func applyBootstrapBot(ctx context.Context, logger *log.Logger, c *core.ChattoCore, spec config.BootstrapBot) bool {
	if spec.Login == "" || spec.OwnerLogin == "" || spec.CredentialFile == "" {
		logger.Error("Skipping [bootstrap] bot with missing login, owner_login, or credential_file")
		return false
	}

	owner, err := c.GetUserByLogin(ctx, spec.OwnerLogin)
	if err != nil || owner == nil || owner.GetIsBot() {
		logger.Error("Skipping [bootstrap] bot because its human owner was not found")
		return false
	}
	if existing, err := c.GetUserByLogin(ctx, spec.Login); err == nil && existing != nil {
		logger.Error("Skipping [bootstrap] bot because its login is already in use", "user_id", existing.GetId())
		return false
	}

	rooms, err := c.ListRooms(ctx, core.KindChannel)
	if err != nil {
		logger.Error("Skipping [bootstrap] bot because rooms could not be listed", "error", err)
		return false
	}
	roomsByName := make(map[string]string, len(rooms))
	for _, room := range rooms {
		roomsByName[room.GetName()] = room.GetId()
	}
	for _, roomName := range spec.Rooms {
		if roomsByName[roomName] == "" {
			logger.Error("Skipping [bootstrap] bot because a configured room does not exist", "room", roomName)
			return false
		}
	}
	for _, permission := range spec.Permissions {
		if err := core.ValidatePermission(core.Permission(permission)); err != nil {
			logger.Error("Skipping [bootstrap] bot because a configured permission is invalid", "permission", permission, "error", err)
			return false
		}
	}

	displayName := spec.DisplayName
	if displayName == "" {
		displayName = spec.Login
	}
	bot, err := c.CreateBotWithAPIKeyName(ctx, owner.GetId(), spec.Login, displayName, spec.APIKeyName)
	if err != nil {
		logger.Error("Failed to create [bootstrap] bot", "error", err)
		return false
	}

	configured := true
	for _, permission := range spec.Permissions {
		if err := c.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), core.PermissionTargetScope{Kind: core.MatrixScopeServer}, core.Permission(permission), core.PermissionStateAllow); err != nil {
			configured = false
			logger.Error("Failed to delegate permission to [bootstrap] bot", "user_id", bot.User.GetId(), "permission", permission, "error", err)
		}
	}
	for _, roomName := range spec.Rooms {
		if _, err := c.JoinRoom(ctx, bot.User.GetId(), core.KindChannel, bot.User.GetId(), roomsByName[roomName]); err != nil {
			configured = false
			logger.Error("Failed to join [bootstrap] bot to room", "user_id", bot.User.GetId(), "room", roomName, "error", err)
		}
	}
	if err := writeBootstrapCredential(spec.CredentialFile, bot.APIKey); err != nil {
		logger.Error("Failed to write [bootstrap] bot credential", "user_id", bot.User.GetId(), "error", err)
		return false
	}
	if !configured {
		logger.Warn("Created [bootstrap] bot with incomplete permissions or membership", "user_id", bot.User.GetId())
		return false
	}
	logger.Info("Created [bootstrap] bot", "user_id", bot.User.GetId())
	return true
}

// writeBootstrapCredential atomically replaces a development credential file.
// Both the temporary file and final file use owner-only permissions.
func writeBootstrapCredential(path, credential string) error {
	if path == "" {
		return errors.New("credential file path is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".chatto-bootstrap-credential-*")
	if err != nil {
		return fmt.Errorf("create temporary credential file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set credential file permissions: %w", err)
	}
	if _, err := temporary.WriteString(credential + "\n"); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write credential file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close credential file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace credential file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("set final credential file permissions: %w", err)
	}
	return nil
}

// serverDataEmptyForBootstrap returns true when only built-in first-boot
// scaffolding exists. core.Run creates the seed Lobby group and SeedDefaultRooms
// creates announcements/general before the dev bootstrap hook fires, so those
// system-owned defaults do not count as data that should block bootstrap.
func serverDataEmptyForBootstrap(ctx context.Context, c *core.ChattoCore) (bool, error) {
	userCount, err := c.CountUsers(ctx)
	if err != nil {
		return false, err
	}
	if userCount > 0 {
		return false, nil
	}

	if cm := c.ConfigModel(); cm != nil {
		if cfg := cm.GetServerConfig(); cfg != nil {
			return false, nil
		}
	}

	rooms, err := c.ListRooms(ctx, core.KindChannel)
	if err != nil {
		return false, err
	}
	defaultRoomNames := map[string]struct{}{}
	for _, room := range core.DefaultGlobalRooms {
		defaultRoomNames[room.Name] = struct{}{}
	}
	for _, room := range rooms {
		if _, ok := defaultRoomNames[room.Name]; !ok {
			return false, nil
		}
	}

	groups, err := c.ListRoomGroupsOrdered(ctx, core.KindChannel)
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		if group.Name != core.SeedDefaultRoomGroupName {
			return false, nil
		}
	}
	return true, nil
}

// applyBootstrapUser creates the user if missing, sets a verified email if the
// section has one, and assigns an role if specified. Returns the
// resolved user ID (whether existing or newly created) and whether we created it.
func applyBootstrapUser(ctx context.Context, logger *log.Logger, c *core.ChattoCore, u config.BootstrapUser) (string, bool) {
	if u.Login == "" {
		logger.Error("Skipping [bootstrap] user with empty login")
		return "", false
	}

	if existing, err := c.GetUserByLogin(ctx, u.Login); err == nil && existing != nil {
		logger.Debug("[bootstrap] user already exists; skipping create", "user_id", existing.Id)
		// Still try to apply role + email below (idempotent).
		assignBootstrapRole(ctx, logger, c, existing.Id, u.RoleOrDefault())
		ensureBootstrapEmail(ctx, logger, c, existing.Id, u.Email)
		return existing.Id, false
	}

	displayName := u.DisplayName
	if displayName == "" {
		displayName = u.Login
	}

	user, err := c.CreateUser(ctx, "system", u.Login, displayName, u.Password)
	if err != nil {
		logger.Error("Failed to create [bootstrap] user", "error", err)
		return "", false
	}

	ensureBootstrapEmail(ctx, logger, c, user.Id, u.Email)
	assignBootstrapRole(ctx, logger, c, user.Id, u.RoleOrDefault())

	return user.Id, true
}

func ensureBootstrapEmail(ctx context.Context, logger *log.Logger, c *core.ChattoCore, userID, email string) {
	if email == "" {
		return
	}
	if err := c.AddVerifiedEmailDirect(ctx, userID, email); err != nil {
		// ErrEmailAlreadyVerified is fine — the email is already attached.
		if !errors.Is(err, core.ErrEmailAlreadyVerified) {
			logger.Warn("Failed to add verified email for [bootstrap] user", "user_id", userID, "error", err)
		}
	}
}

func assignBootstrapRole(ctx context.Context, logger *log.Logger, c *core.ChattoCore, userID, role string) {
	if role == "" {
		return
	}
	var roleName string
	switch role {
	case "owner":
		roleName = core.RoleOwner
	case "admin":
		roleName = core.RoleAdmin
	case "moderator":
		roleName = core.RoleModerator
	default:
		logger.Warn("Unknown server_role in [bootstrap]; ignoring", "user_id", userID, "role", role)
		return
	}
	// SystemActorID is trusted bootstrap context rather than a user action.
	if err := c.AssignServerRole(ctx, core.SystemActorID, userID, roleName); err != nil {
		logger.Warn("Failed to assign role for [bootstrap] user", "user_id", userID, "role", role, "error", err)
	}
}

// applyBootstrapServer seeds the server's user-visible config (name)
// and ensures the deployment's primary room group exists. The underlying
// primary-space record is a transitional storage detail (per ADR-027 the
// data model still routes through a Space until PR(c) collapses the RBAC
// engines) — operators don't configure or see it directly. Returns true if
// a primary space was newly created, false otherwise (already-existing or
// skipped).
func applyBootstrapServer(ctx context.Context, logger *log.Logger, c *core.ChattoCore, inst config.BootstrapServer, ownerID string, bootstrapUserIDs []string) bool {
	if inst.Name == "" {
		logger.Error("Skipping [bootstrap.server] with empty name")
		return false
	}

	// Seed the runtime server config (idempotent — only writes when the
	// name field is unset, so an admin-edited server name isn't clobbered
	// on every dev restart).
	if cm := c.ConfigModel(); cm != nil {
		current := cm.GetServerConfig()
		if current == nil || current.ServerName == "" {
			if _, err := cm.UpdateServerConfigFunc(ctx, "system:bootstrap", func(current *configv1.ServerConfig) (*configv1.ServerConfig, error) {
				if current == nil {
					return &configv1.ServerConfig{ServerName: inst.Name}, nil
				}
				if current.ServerName == "" {
					current.ServerName = inst.Name
				}
				return current, nil
			}); err != nil {
				logger.Warn("Failed to seed server config from [bootstrap.server]", "error", err)
			}
		}
	}

	// Create operator-specified extra rooms (if any). The default rooms
	// (`announcements`, `general`) are seeded by `SeedDefaultRooms` during
	// startup — bootstrap no longer duplicates that. Owner auto-joins
	// every existing channel room so dev/e2e users land ready to use the
	// server.
	for _, name := range inst.Rooms {
		if _, err := c.CreateRoom(ctx, ownerID, core.KindChannel, "", name, ""); err != nil {
			if !errors.Is(err, core.ErrRoomNameExists) {
				logger.Warn("Failed to create [bootstrap] room", "room", name, "error", err)
			}
		}
	}

	existing, err := c.ListRooms(ctx, core.KindChannel)
	if err != nil {
		logger.Warn("Failed to list rooms for bootstrap owner auto-join", "error", err)
	}
	for _, userID := range bootstrapUserIDs {
		for _, room := range existing {
			if _, err := c.JoinRoom(ctx, userID, core.KindChannel, userID, room.Id); err != nil {
				logger.Warn("Failed to auto-join bootstrap user to room",
					"user_id", userID, "room", room.Name, "error", err)
			}
		}
	}

	return true
}
