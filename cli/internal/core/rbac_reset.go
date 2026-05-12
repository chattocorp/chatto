package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core/rbac"
)

// ResetRBAC wipes the SERVER_RBAC bucket, re-seeds the system roles plus
// default permissions from code, and assigns the `owner` role to every user
// whose verified email matches `owners.emails` in the supplied config.
//
// This is the operator escape hatch for misconfigured / drifted RBAC state
// and the upgrade tool for moving an existing deployment onto the unified
// Phase-5 server-RBAC layout. Idempotent: running it twice produces the
// same result.
//
// Wiping is intentionally aggressive — every key in SERVER_RBAC is deleted,
// including custom roles and any room-level permission overrides. Rebuild
// those after the reset.
func (c *ChattoCore) ResetRBAC(ctx context.Context, ownersCfg config.OwnersConfig) error {
	kv := c.storage.serverRBACKV

	// Drain all keys.
	keyLister, err := kv.ListKeys(ctx)
	if err != nil {
		return fmt.Errorf("list SERVER_RBAC keys: %w", err)
	}
	deleted := 0
	for key := range keyLister.Keys() {
		if err := kv.Purge(ctx, key); err != nil {
			return fmt.Errorf("purge key %q: %w", key, err)
		}
		deleted++
	}
	c.logger.Info("Wiped SERVER_RBAC", "keys_deleted", deleted)

	// Re-create the system roles. The everyone role stays virtual.
	engine := c.storage.serverRBACEngine
	roleSpecs := []struct {
		name        string
		displayName string
		description string
		position    int32
	}{
		{RoleOwner, "Owner", "Full server control", rbac.PositionOwner},
		{RoleAdmin, "Admin", "Full administrative access", rbac.PositionAdmin},
		{RoleModerator, "Moderator", "Moderation permissions without administrative reach", rbac.PositionModerator},
	}
	for _, spec := range roleSpecs {
		if _, err := engine.CreateRoleWithPosition(ctx, spec.name, spec.displayName, spec.description, spec.position); err != nil {
			if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
				return fmt.Errorf("create role %q: %w", spec.name, err)
			}
		}
	}

	// Seed default permissions. Owner gets everything. Admin / moderator /
	// everyone get the documented default sets. We funnel both the
	// instance-tier and space-tier defaults into the unified bucket so the
	// resulting permission grants cover what each role used to have across
	// the dual-tier model.
	if err := c.InitInstanceDefaults(ctx); err != nil {
		return fmt.Errorf("seed instance defaults: %w", err)
	}
	// Seed the channel-scope defaults into the same unified RBAC bucket.
	if err := c.InitSpaceDefaults(ctx); err != nil {
		return fmt.Errorf("seed space defaults: %w", err)
	}

	// Re-write the sentinel so the boot-time guard skips re-seeding next start.
	if _, err := kv.Put(ctx, rbacDefaultsSentinel, []byte("1")); err != nil {
		return fmt.Errorf("write sentinel: %w", err)
	}

	// Auto-promote config owners. Every user whose verified email matches
	// owners.emails in chatto.toml gets the `owner` role.
	users, err := c.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	promoted := 0
	for _, user := range users {
		emails, err := c.GetVerifiedEmails(ctx, user.Id)
		if err != nil {
			c.logger.Warn("Failed to read verified emails for user during RBAC reset",
				"user_id", user.Id, "error", err)
			continue
		}
		matched := false
		for _, ve := range emails {
			if ownersCfg.IsInstanceOwnerEmail(ve.Email) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if err := engine.AssignRole(ctx, user.Id, RoleOwner); err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				continue
			}
			return fmt.Errorf("assign owner role to %s: %w", user.Id, err)
		}
		c.logger.Info("Promoted config owner to owner role",
			"user_id", user.Id, "login", user.Login)
		promoted++
	}

	c.logger.Info("RBAC reset complete",
		"roles_seeded", len(roleSpecs),
		"owners_promoted", promoted)
	return nil
}
