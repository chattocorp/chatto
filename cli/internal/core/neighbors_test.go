package core

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/config"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestNeighborConcurrentDuplicateAcrossReplicas(t *testing.T) {
	first, nc := setupTestCore(t)
	ctx := testContext(t)
	second, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	require.NoError(t, err)
	startCoreServices(t, second)

	actor, err := first.CreateUser(ctx, SystemActorID, "neighbor-replica-manager", "Neighbor Replica Manager", "password123")
	require.NoError(t, err)
	require.NoError(t, first.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermServerManageNeighbors))

	errorsByReplica := make(chan error, 2)
	for _, replica := range []*ChattoCore{first, second} {
		go func(replica *ChattoCore) {
			_, createErr := replica.CreateNeighbor(ctx, actor.GetId(), "https://same.example", "")
			errorsByReplica <- createErr
		}(replica)
	}
	var successes, duplicates int
	for range 2 {
		switch err := <-errorsByReplica; {
		case err == nil:
			successes++
		case errors.Is(err, ErrNeighborAlreadyExists):
			duplicates++
		default:
			t.Fatalf("concurrent CreateNeighbor error = %v", err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, duplicates)
}

func TestNeighborConcurrentUpdatesAcrossReplicas(t *testing.T) {
	first, nc := setupTestCore(t)
	ctx := testContext(t)
	second, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	require.NoError(t, err)
	startCoreServices(t, second)

	actor, err := first.CreateUser(ctx, SystemActorID, "neighbor-update-manager", "Neighbor Update Manager", "password123")
	require.NoError(t, err)
	require.NoError(t, first.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermServerManageNeighbors))

	one, err := first.CreateNeighbor(ctx, actor.GetId(), "https://one.example", "")
	require.NoError(t, err)
	two, err := first.CreateNeighbor(ctx, actor.GetId(), "https://two.example", "")
	require.NoError(t, err)

	type updateResult struct {
		neighbor Neighbor
		err      error
	}
	results := make(chan updateResult, 2)
	go func() {
		neighbor, updateErr := first.UpdateNeighbor(ctx, actor.GetId(), one.ID, "https://one-updated.example", nil, one.Revision)
		results <- updateResult{neighbor: neighbor, err: updateErr}
	}()
	go func() {
		neighbor, updateErr := second.UpdateNeighbor(ctx, actor.GetId(), two.ID, "https://two-updated.example", nil, two.Revision)
		results <- updateResult{neighbor: neighbor, err: updateErr}
	}()
	for range 2 {
		result := <-results
		require.NoError(t, result.err)
		require.Contains(t, result.neighbor.Origin, "-updated.example")
	}

	target, err := first.CreateNeighbor(ctx, actor.GetId(), "https://contended.example", "")
	require.NoError(t, err)
	for index, replica := range []*ChattoCore{first, second} {
		go func(index int, replica *ChattoCore) {
			neighbor, updateErr := replica.UpdateNeighbor(ctx, actor.GetId(), target.ID, fmt.Sprintf("https://winner-%d.example", index), nil, target.Revision)
			results <- updateResult{neighbor: neighbor, err: updateErr}
		}(index, replica)
	}
	var successes, stale int
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			successes++
		case errors.Is(result.err, ErrNeighborRevisionChanged):
			stale++
		default:
			t.Fatalf("concurrent UpdateNeighbor error = %v", result.err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, stale)
}

func TestNeighborCRUDAndPermission(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	actor, err := chatto.CreateUser(ctx, SystemActorID, "neighbor-manager", "Neighbor Manager", "password123")
	require.NoError(t, err)

	_, err = chatto.ListManagedNeighbors(ctx, actor.GetId())
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.NoError(t, chatto.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermServerManageNeighbors))

	created, err := chatto.CreateNeighbor(ctx, actor.GetId(), " HTTPS://Example.COM:443/ ", " A thoughtful place to talk. ")
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)
	require.Equal(t, "https://example.com", created.Origin)
	require.Equal(t, "A thoughtful place to talk.", created.Testimonial)
	require.NotEmpty(t, created.Revision)

	neighbors, err := chatto.ListManagedNeighbors(ctx, actor.GetId())
	require.NoError(t, err)
	require.Equal(t, []Neighbor{created}, neighbors)

	_, err = chatto.CreateNeighbor(ctx, actor.GetId(), "https://example.com", "")
	require.ErrorIs(t, err, ErrNeighborAlreadyExists)

	updatedTestimonial := "A welcoming place for makers."
	updated, err := chatto.UpdateNeighbor(ctx, actor.GetId(), created.ID, "http://chat.example:8080", &updatedTestimonial, created.Revision)
	require.NoError(t, err)
	require.Equal(t, "http://chat.example:8080", updated.Origin)
	require.Equal(t, updatedTestimonial, updated.Testimonial)
	require.NotEqual(t, created.Revision, updated.Revision)

	originOnly, err := chatto.UpdateNeighbor(ctx, actor.GetId(), created.ID, "https://origin-only.example", nil, updated.Revision)
	require.NoError(t, err)
	require.Equal(t, updatedTestimonial, originOnly.Testimonial)

	clearedTestimonial := ""
	cleared, err := chatto.UpdateNeighbor(ctx, actor.GetId(), created.ID, originOnly.Origin, &clearedTestimonial, originOnly.Revision)
	require.NoError(t, err)
	require.Empty(t, cleared.Testimonial)

	_, err = chatto.UpdateNeighbor(ctx, actor.GetId(), created.ID, "https://stale.example", nil, created.Revision)
	require.ErrorIs(t, err, ErrNeighborRevisionChanged)
	require.ErrorIs(t, chatto.DeleteNeighbor(ctx, actor.GetId(), created.ID, created.Revision), ErrNeighborRevisionChanged)
	require.NoError(t, chatto.DeleteNeighbor(ctx, actor.GetId(), created.ID, cleared.Revision))
	_, err = chatto.GetManagedNeighbor(ctx, actor.GetId(), created.ID)
	require.ErrorIs(t, err, ErrNeighborNotFound)
}

func TestNeighborRejectsServerOrigins(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	actor, err := chatto.CreateUser(ctx, SystemActorID, "self-neighbor-manager", "Self Neighbor Manager", "password123")
	require.NoError(t, err)
	require.NoError(t, chatto.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermServerManageNeighbors))

	chatto.serverOrigins = nil
	historical, err := chatto.CreateNeighbor(ctx, actor.GetId(), "https://self.example", "")
	require.NoError(t, err)
	external, err := chatto.CreateNeighbor(ctx, actor.GetId(), "https://external.example", "")
	require.NoError(t, err)

	chatto.serverOrigins = map[string]struct{}{
		"https://self.example":  {},
		"https://alias.example": {},
	}

	_, err = chatto.CreateNeighbor(ctx, actor.GetId(), " HTTPS://SELF.EXAMPLE:443/ ", "")
	require.ErrorIs(t, err, ErrNeighborMatchesServerOrigin)
	_, err = chatto.CreateNeighbor(ctx, actor.GetId(), "https://alias.example", "")
	require.ErrorIs(t, err, ErrNeighborMatchesServerOrigin)

	_, err = chatto.UpdateNeighbor(ctx, actor.GetId(), external.ID, "https://alias.example", nil, external.Revision)
	require.ErrorIs(t, err, ErrNeighborMatchesServerOrigin)
	unchanged, err := chatto.GetManagedNeighbor(ctx, actor.GetId(), external.ID)
	require.NoError(t, err)
	require.Equal(t, external, unchanged)

	corrected, err := chatto.UpdateNeighbor(ctx, actor.GetId(), historical.ID, "https://corrected.example", nil, historical.Revision)
	require.NoError(t, err)
	require.Equal(t, "https://corrected.example", corrected.Origin)
	require.Len(t, chatto.ConfigModel().ListNeighbors(), 2)
}

func TestServerManageIncludesNeighborManagement(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	actor, err := chatto.CreateUser(ctx, SystemActorID, "server-manager-neighbor", "Server Manager", "password123")
	require.NoError(t, err)
	require.NoError(t, chatto.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermServerManage))

	_, err = chatto.CreateNeighbor(ctx, actor.GetId(), "https://included.example", "")
	require.NoError(t, err)
}

func TestCanonicalNeighborOrigin(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"http://Example.COM:80/", "http://example.com"},
		{"https://example.com:0443", "https://example.com"},
		{"https://BÜCHER.example", "https://xn--bcher-kva.example"},
		{"https://[2001:db8::1]:443", "https://[2001:db8::1]"},
		{"http://localhost:3000", "http://localhost:3000"},
	}
	for _, test := range tests {
		got, err := canonicalNeighborOrigin(test.raw)
		require.NoError(t, err, test.raw)
		require.Equal(t, test.want, got, test.raw)
	}

	invalid := []string{"", "example.com", "ftp://example.com", "https://user@example.com", "https://example.com/path", "https://example.com?x=1", "https://example.com#fragment", "https://example.com:0", "https://example.com:"}
	for _, raw := range invalid {
		_, err := canonicalNeighborOrigin(raw)
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("canonicalNeighborOrigin(%q) error = %v, want ErrInvalidArgument", raw, err)
		}
	}
}

func TestNormalizeNeighborTestimonial(t *testing.T) {
	got, err := normalizeNeighborTestimonial("  A kind place.\n")
	require.NoError(t, err)
	require.Equal(t, "A kind place.", got)

	_, err = normalizeNeighborTestimonial(strings.Repeat("界", MaxNeighborTestimonialLength+1))
	require.ErrorIs(t, err, ErrInvalidArgument)
}

func TestConfigProjectionNeighborSnapshotRoundTrip(t *testing.T) {
	projection := NewConfigProjection()
	require.NoError(t, projection.Apply(newNeighborCreatedProjectionEvent("E-create", "N1", "https://one.example", "First testimonial"), 1))
	require.NoError(t, projection.Apply(newNeighborCreatedProjectionEvent("E-create-2", "N2", "https://two.example", ""), 2))
	require.NoError(t, projection.Apply(&evtv1.Event{Id: "E-testimonial", Event: &evtv1.Event_ServerNeighborTestimonialChanged{
		ServerNeighborTestimonialChanged: &evtv1.ServerNeighborTestimonialChangedEvent{NeighborId: "N1", Testimonial: "Updated testimonial"},
	}}, 3))

	payload, err := projection.Snapshot()
	require.NoError(t, err)
	restored := NewConfigProjection()
	require.NoError(t, restored.Restore(payload))
	require.ElementsMatch(t, NewConfigModel(nil, detachedTestProjectionHandle(projection)).ListNeighbors(), NewConfigModel(nil, detachedTestProjectionHandle(restored)).ListNeighbors())
}
