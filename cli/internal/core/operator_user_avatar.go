package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// MaxHistoricalAvatarBytes bounds one avatar image, not the archive.
const MaxHistoricalAvatarBytes = 5 << 20

// ImportHistoricalUserAvatar fills an absent avatar with permanent retry
// protection. Existing avatars win, including changes concurrent with import.
// A skipped assignment also commits a receipt, so a retry after a later clear
// cannot restore the imported avatar.
func (c *ChattoCore) ImportHistoricalUserAvatar(ctx context.Context, key, userID string, image []byte) error {
	if len(image) == 0 || len(image) > MaxHistoricalAvatarBytes {
		return invalidArgument("avatar image must contain 1 byte to 5 MiB")
	}
	sum := sha256.Sum256(image)
	operation, err := c.operatorOperation(ctx, key, "ImportUserAvatar", []string{userID, hex.EncodeToString(sum[:])})
	if err != nil || operation.resource != "" {
		return err
	}
	if _, err = c.GetUser(ctx, userID); err != nil {
		return err
	}
	current, err := c.GetUserAvatar(ctx, userID)
	if err != nil {
		return err
	}
	var asset *evtv1.AssetRecord
	if current == nil {
		asset, err = c.storeUserAvatarAsset(ctx, userID, bytes.NewReader(image))
		if err != nil {
			return err
		}
	}
	used := false
	_, err = c.appendUserBatchWithMentionableCompose(ctx, userID, maxUserMutationRetries, func() ([]evtstream.BatchEntry, error) {
		if _, err := c.GetUser(ctx, userID); err != nil {
			return nil, err
		}
		current, err := c.GetUserAvatar(ctx, userID)
		if err != nil {
			return nil, err
		}
		entries := []evtstream.BatchEntry{}
		used = current == nil && asset != nil
		if !used {
			// A receipt-only batch must keep its create-once subject guard.
			// The user batch helper replaces the first entry's guard with OCC.
			return nil, nil
		}
		event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_AssetCreated{AssetCreated: &evtv1.AssetCreatedEvent{Asset: asset, OriginalBinaryAvailable: true, UserId: userID}}, ImportContext: &evtv1.ImportContext{HiddenTimeline: true, UnknownTime: true}})
		entries = append(entries, evtstream.BatchEntry{Subject: evtstream.UserAggregate(userID).SubjectFor(event), Event: event})
		return append(entries, operation.receipt(asset.GetId())), nil
	})
	if err == nil && !used {
		// Preservation changes no user state, so it needs only the receipt's
		// uniqueness guard. A later avatar clear must not undo this decision.
		_, err = c.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{operation.receipt(userID)})
	}
	if err != nil {
		// Retain uploaded bytes on ambiguous commits. Deleting them could break
		// an avatar that another replica already observes.
		if loadErr := operation.load(ctx); loadErr != nil {
			return loadErr
		}
		if operation.resource != "" {
			// A competing receipt proves our distinct upload did not commit.
			if asset != nil && operation.resource != asset.GetId() {
				c.CleanupAsset(ctx, DeprecatedAssetFromAsset(asset))
			}
			return nil
		}
		return err
	}
	if asset != nil && !used {
		c.CleanupAsset(ctx, DeprecatedAssetFromAsset(asset))
	}
	return nil
}
