package graph

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/core"
)

func TestAdminProjections_State(t *testing.T) {
	env := setupTestResolver(t)
	ctx := env.authContext()

	projections, err := env.resolver.AdminQueries().Projections(ctx, nil)
	require.NoError(t, err)
	require.NotEmpty(t, projections)

	byName := make(map[string]bool, len(projections))
	for _, p := range projections {
		byName[p.Name] = true
		require.NotEmpty(t, p.Subjects, "projection %s should expose subject filters", p.Name)
		require.NotEmpty(t, p.LastAppliedSequence)
		require.NotEmpty(t, p.MatchingStreamSequence)
		require.NotEmpty(t, p.StreamLastSequence)
		require.NotNil(t, p.Metrics)
		require.GreaterOrEqual(t, p.EstimatedBytes, 0)
		require.GreaterOrEqual(t, p.AverageEntryBytes, 0)
		require.GreaterOrEqual(t, p.Lag, 0)
	}
	require.True(t, byName["Room Timeline"])
	require.True(t, byName["Threads"])
	require.True(t, byName["Reactions"])
}

func TestAdminProjections_AuthorizationDenied(t *testing.T) {
	env := setupTestResolver(t)
	regular := env.createVerifiedUser(t, "no-system-view", "No System View", "password123")

	_, err := env.resolver.AdminQueries().Projections(env.authContextForUser(regular), nil)
	require.True(t, errors.Is(err, core.ErrPermissionDenied), "Projections should deny non-system admins, got: %v", err)
}
