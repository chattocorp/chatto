package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
)

type mentionHandleOwnerKind string

const (
	mentionHandleOwnerUser mentionHandleOwnerKind = "user"
	mentionHandleOwnerRole mentionHandleOwnerKind = "role"
)

type mentionHandleClaim struct {
	Kind mentionHandleOwnerKind `json:"kind"`
	ID   string                 `json:"id"`
}

type mentionHandleClaimedError struct {
	owner mentionHandleClaim
}

func (e *mentionHandleClaimedError) Error() string {
	if e.owner.Kind == "" || e.owner.ID == "" {
		return "mention handle is already claimed"
	}
	return fmt.Sprintf("mention handle is already claimed by %s %q", e.owner.Kind, e.owner.ID)
}

func (c *ChattoCore) claimUserMentionHandle(ctx context.Context, login, userID string) (bool, error) {
	created, err := c.claimMentionHandle(ctx, login, mentionHandleClaim{
		Kind: mentionHandleOwnerUser,
		ID:   userID,
	})
	if err == nil {
		return created, nil
	}

	var claimed *mentionHandleClaimedError
	if errors.As(err, &claimed) && claimed.owner.Kind == mentionHandleOwnerUser {
		return false, ErrLoginAlreadyTaken
	}
	if errors.As(err, &claimed) {
		return false, ErrUsernameBlocked
	}
	return false, err
}

func (c *ChattoCore) claimRoleMentionHandle(ctx context.Context, roleName string) (bool, error) {
	created, err := c.claimMentionHandle(ctx, roleName, mentionHandleClaim{
		Kind: mentionHandleOwnerRole,
		ID:   strings.ToLower(roleName),
	})
	if err == nil {
		return created, nil
	}

	var claimed *mentionHandleClaimedError
	if errors.As(err, &claimed) {
		return false, ErrRoleAlreadyExists
	}
	return false, err
}

func (c *ChattoCore) claimMentionHandle(ctx context.Context, handle string, owner mentionHandleClaim) (bool, error) {
	normalized := normalizeMentionHandleClaim(handle)
	if normalized == "" || IsVirtualMentionHandle(normalized) {
		return false, &mentionHandleClaimedError{}
	}

	data, err := json.Marshal(owner)
	if err != nil {
		return false, fmt.Errorf("marshal mention handle claim: %w", err)
	}

	key := mentionHandleClaimKey(normalized)
	if _, err := c.storage.runtimeStateKV.Create(ctx, key, data); err == nil {
		return true, nil
	} else if !errors.Is(err, jetstream.ErrKeyExists) {
		return false, fmt.Errorf("claim mention handle %q: %w", normalized, err)
	}

	existing, err := c.mentionHandleClaimOwner(ctx, key)
	if err != nil {
		return false, err
	}
	if existing.Kind == owner.Kind && existing.ID == owner.ID {
		return false, nil
	}
	return false, &mentionHandleClaimedError{owner: existing}
}

func (c *ChattoCore) releaseMentionHandle(ctx context.Context, handle string, owner mentionHandleClaim) error {
	key := mentionHandleClaimKey(normalizeMentionHandleClaim(handle))
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read mention handle claim %q: %w", handle, err)
	}
	if !bytes.Equal(entry.Value(), mustMarshalMentionHandleClaim(owner)) {
		return nil
	}
	if err := c.storage.runtimeStateKV.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("release mention handle %q: %w", handle, err)
	}
	return nil
}

func (c *ChattoCore) mentionHandleClaimOwner(ctx context.Context, key string) (mentionHandleClaim, error) {
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return mentionHandleClaim{}, &mentionHandleClaimedError{}
		}
		return mentionHandleClaim{}, fmt.Errorf("read mention handle claim %q: %w", key, err)
	}

	var owner mentionHandleClaim
	if err := json.Unmarshal(entry.Value(), &owner); err != nil {
		return mentionHandleClaim{}, fmt.Errorf("decode mention handle claim %q: %w", key, err)
	}
	return owner, nil
}

func mustMarshalMentionHandleClaim(owner mentionHandleClaim) []byte {
	data, err := json.Marshal(owner)
	if err != nil {
		panic(err)
	}
	return data
}

func mentionHandleClaimKey(normalizedHandle string) string {
	return "mention_handle." + normalizedHandle
}

func normalizeMentionHandleClaim(handle string) string {
	return strings.ToLower(strings.TrimSpace(handle))
}
