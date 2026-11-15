package core

import (
	"context"
	"fmt"
	"regexp"

	"github.com/Masterminds/semver"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

var releaseHighlightID = regexp.MustCompile(`^(users|admins):[a-z0-9][a-z0-9._-]*$`)

// DismissReleaseHighlights adds private acknowledgements without replacing
// concurrent changes. The transport must authenticate userID. IDs are authored
// by clients, so a server does not need the same article catalog as its client.
// Each request and account set is bounded to limit storage and viewer payloads.
func (c *ChattoCore) DismissReleaseHighlights(ctx context.Context, userID string, ids []string) (*evtv1.ServerUserPreferences, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, fmt.Errorf("%w: expected 1 to 100 highlight IDs", ErrInvalidArgument)
	}
	for _, id := range ids {
		if len(id) > 128 || !releaseHighlightID.MatchString(id) {
			return nil, fmt.Errorf("%w: invalid highlight ID", ErrInvalidArgument)
		}
	}
	if c.configModel == nil {
		return nil, fmt.Errorf("config model not configured")
	}
	err := c.configModel.updateSubject(ctx, userID, func(_ evtstream.Aggregate, _ string, _ uint64) ([]*evtv1.Event, error) {
		current, _ := c.configModel.userSettings(userID)
		seen := make(map[string]struct{})
		for _, id := range current.GetDismissedReleaseHighlights() {
			seen[id] = struct{}{}
		}
		var added []string
		for _, id := range ids {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				added = append(added, id)
			}
		}
		if len(added) == 0 {
			return nil, nil
		}
		if len(seen) > 4096 {
			return nil, ErrLimitExceeded
		}
		return []*evtv1.Event{newEvent(userID, &evtv1.Event{Event: &evtv1.Event_UserReleaseHighlightsChanged{UserReleaseHighlightsChanged: &evtv1.UserReleaseHighlightsChangedEvent{UserId: userID, Ids: added}}})}, nil
	})
	if err != nil {
		return nil, err
	}
	return c.GetUserSettings(ctx, userID)
}

// InitializeReleaseHighlights establishes a first-write-wins baseline per surface.
// It records no dismissals. Callers must authenticate userID. Version metadata is
// client-authored because standalone frontends can be newer than this server.
func (c *ChattoCore) InitializeReleaseHighlights(ctx context.Context, userID, surface, version string) (*evtv1.ServerUserPreferences, error) {
	if surface != "users" && surface != "admins" {
		return nil, ErrInvalidArgument
	}
	if len(version) > 128 {
		return nil, ErrInvalidArgument
	}
	parsed, err := semver.NewVersion(version)
	if err != nil || parsed.String() != version {
		return nil, fmt.Errorf("%w: expected a full Semantic Version", ErrInvalidArgument)
	}
	if c.configModel == nil {
		return nil, fmt.Errorf("config model not configured")
	}
	err = c.configModel.updateSubject(ctx, userID, func(_ evtstream.Aggregate, _ string, _ uint64) ([]*evtv1.Event, error) {
		current, _ := c.configModel.userSettings(userID)
		change := &evtv1.UserReleaseHighlightsChangedEvent{UserId: userID}
		if surface == "users" {
			if current != nil && current.HighlightsUsersSinceVersion != nil {
				return nil, nil
			}
			change.UsersSinceVersion = &version
		} else {
			if current != nil && current.HighlightsAdminsSinceVersion != nil {
				return nil, nil
			}
			change.AdminsSinceVersion = &version
		}
		return []*evtv1.Event{newEvent(userID, &evtv1.Event{Event: &evtv1.Event_UserReleaseHighlightsChanged{UserReleaseHighlightsChanged: change}})}, nil
	})
	if err != nil {
		return nil, err
	}
	return c.GetUserSettings(ctx, userID)
}
