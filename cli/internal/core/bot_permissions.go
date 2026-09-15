package core

import (
	"context"
	"slices"
)

// BotPermission describes a configured permission at its effective scope.
// Inactive entries are returned only to bot managers. Room membership and
// operation-specific requirements remain separate from permission grants.
type BotPermission struct {
	Permission Permission
	Scope      PermissionMatrixScope
	Active     bool
}

// ListBotPermissions describes a bot's effective allowlist to authenticated
// members. It reads one content view, compresses equivalent scopes, and removes
// room metadata the viewer cannot see. No credential metadata is exposed.
func (c *ChattoCore) ListBotPermissions(ctx context.Context, actorID, botID string) ([]BotPermission, error) {
	var result []BotPermission
	err := c.ReadServerContentView(ctx, func(ctx context.Context, _ uint64) error {
		result = nil
		actorIsBot, _, actorExists := c.userModel.isBotAndOwner(actorID)
		if !actorExists {
			return ErrNotFound
		}
		isBot, ownerID, botExists := c.userModel.isBotAndOwner(botID)
		if !botExists || !isBot {
			return ErrNotFound
		}
		manager := !actorIsBot && actorID == ownerID
		if !actorIsBot && !manager {
			allowed, err := c.CanManageBots(ctx, actorID)
			if err != nil {
				return err
			}
			manager = allowed
		}
		scopes, err := c.buildMatrixScopes(ctx, true)
		if err != nil {
			return err
		}
		// A stable scope order also gives stable pagination after compression.
		slices.SortFunc(scopes, func(a, b PermissionMatrixScope) int {
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
		visible := make(map[string]bool, len(scopes))
		for _, scope := range scopes {
			visible[scope.ID] = true
			if scope.Kind == MatrixScopeRoom {
				allowed, err := c.CanSeeRoom(ctx, actorID, KindChannel, scopeRefID(scope.ID, "room:"))
				if err != nil {
					return err
				}
				visible[scope.ID] = allowed
			}
		}
		// Status 0 means unconfigured, 1 active, and 2 configured but unavailable.
		statuses := make(map[Permission]map[string]int)
		for _, meta := range AllPermissions() {
			perm := meta.Permission
			if !botPermissionDelegable(perm) {
				continue
			}
			statuses[perm] = make(map[string]int)
			for _, scope := range scopes {
				cell, applicable, err := c.buildUserPermissionMatrixCell(ctx, botID, perm, scope)
				if err != nil {
					return err
				}
				if !applicable {
					continue
				}
				status := 0
				if cell.Effective == MatrixDecisionAllow {
					status = 1
				} else {
					kind, roomID, groupID := KindChannel, "", ""
					switch scope.Kind {
					case MatrixScopeDM:
						kind = KindDM
					case MatrixScopeRoom:
						roomID, groupID = scopeRefID(scope.ID, "room:"), scope.ParentGroupID
					case MatrixScopeGroup:
						groupID = scopeRefID(scope.ID, "group:")
					}
					if c.PermResolver().botDelegatedDecision(botID, kind, roomID, groupID, perm) == DecisionAllow {
						status = 2
					}
				}
				statuses[perm][scope.ID] = status
			}
		}
		for _, meta := range AllPermissions() {
			states, exists := statuses[meta.Permission]
			if !exists {
				continue
			}
			// Explicit inclusion avoids duplicate bullets for broad read and its
			// interaction-read subset, without hiding independently effective subsets.
			filtered := make(map[string]int, len(states))
			for id, status := range states {
				filtered[id] = status
				for _, including := range includingPermissions(meta.Permission) {
					if status != 0 && statuses[including][id] == status {
						filtered[id] = 0
					}
				}
			}
			for _, entry := range compactBotPermission(meta.Permission, scopes, filtered) {
				if !visible[entry.Scope.ID] || (!entry.Active && !manager) {
					continue
				}
				result = append(result, entry)
			}
		}
		return nil
	})
	return result, err
}

// compactBotPermission emits a broader scope only when every applicable child
// has the same status. Hidden rooms participate in this check: a public global
// claim must not overstate the bot's access in a hidden room.
func compactBotPermission(perm Permission, scopes []PermissionMatrixScope, states map[string]int) []BotPermission {
	// Each child contributes to at most two ancestors, keeping compaction
	// linear in the number of scopes even for large room directories.
	covered := make(map[string]bool, len(states))
	for id, status := range states {
		covered[id] = status != 0
	}
	for _, scope := range scopes {
		status, applicable := states[scope.ID]
		if !applicable {
			continue
		}
		if scope.Kind == MatrixScopeGroup || scope.Kind == MatrixScopeRoom {
			if status != states["server"] {
				covered["server"] = false
			}
		}
		if scope.Kind == MatrixScopeRoom {
			parent := "group:" + scope.ParentGroupID
			if status != states[parent] {
				covered[parent] = false
			}
		}
	}
	var result []BotPermission
	for _, scope := range scopes {
		status := states[scope.ID]
		if status == 0 || !covered[scope.ID] {
			continue
		}
		if (scope.Kind == MatrixScopeGroup || scope.Kind == MatrixScopeRoom) && covered["server"] {
			continue
		}
		if scope.Kind == MatrixScopeRoom && covered["group:"+scope.ParentGroupID] {
			continue
		}
		result = append(result, BotPermission{Permission: perm, Scope: scope, Active: status == 1})
	}
	return result
}
