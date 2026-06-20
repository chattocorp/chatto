package graph

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

func TestBotTokenExpiresAtPresets(t *testing.T) {
	tests := []struct {
		name   string
		preset model.BotTokenExpiryPreset
		want   time.Duration
	}{
		{"30 days", model.BotTokenExpiryPresetDays30, 30 * 24 * time.Hour},
		{"90 days", model.BotTokenExpiryPresetDays90, 90 * 24 * time.Hour},
		{"365 days", model.BotTokenExpiryPresetDays365, 365 * 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now().UTC()
			got, err := botTokenExpiresAt(tt.preset, nil)
			require.NoError(t, err)
			require.NotNil(t, got)
			require.WithinDuration(t, before.Add(tt.want), *got, time.Second)
		})
	}

	got, err := botTokenExpiresAt(model.BotTokenExpiryPresetIndefinite, nil)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestBotTokenExpiresAtCustomValidation(t *testing.T) {
	_, err := botTokenExpiresAt(model.BotTokenExpiryPresetCustom, nil)
	require.True(t, errors.Is(err, core.ErrBotTokenExpired), "missing custom date should be rejected, got %v", err)

	past := timestamppb.New(time.Now().Add(-time.Minute))
	_, err = botTokenExpiresAt(model.BotTokenExpiryPresetCustom, past)
	require.True(t, errors.Is(err, core.ErrBotTokenExpired), "past custom date should be rejected, got %v", err)

	future := time.Now().UTC().Add(3 * 24 * time.Hour)
	got, err := botTokenExpiresAt(model.BotTokenExpiryPresetCustom, timestamppb.New(future))
	require.NoError(t, err)
	require.NotNil(t, got)
	require.WithinDuration(t, future, *got, time.Second)
}

func TestBotTokenResolversCreateListAndRevoke(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()
	query := env.resolver.Query()

	bot, err := mutation.CreateBot(env.authContext(), model.CreateBotInput{
		Login:       "resolverbot",
		DisplayName: "Resolver Bot",
	})
	require.NoError(t, err)
	require.True(t, core.IsBotUser(bot))

	created, err := mutation.CreateBotToken(env.authContext(), model.CreateBotTokenInput{
		BotUserID: bot.Id,
		Name:      "deploy",
		Expiry:    model.BotTokenExpiryPresetDays90,
	})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(created.Secret, "cht_BT"))
	require.NotNil(t, created.Token.ExpiresAt)
	require.WithinDuration(t, time.Now().UTC().Add(90*24*time.Hour), created.Token.ExpiresAt.AsTime(), time.Second)

	tokens, err := query.BotTokens(env.authContext(), bot.Id)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, created.Token.ID, tokens[0].ID)
	require.Nil(t, tokens[0].RevokedAt)
	require.Nil(t, tokens[0].RevokedBy)
	require.Nil(t, tokens[0].RevokeReason)

	ok, err := mutation.RevokeBotToken(env.authContext(), model.RevokeBotTokenInput{
		BotUserID: bot.Id,
		TokenID:   created.Token.ID,
	})
	require.NoError(t, err)
	require.True(t, ok)

	tokens, err = query.BotTokens(env.authContext(), bot.Id)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.NotNil(t, tokens[0].RevokedAt)
	require.NotNil(t, tokens[0].RevokedBy)
	require.Equal(t, env.testUser.Id, tokens[0].RevokedBy.Id)
	require.NotNil(t, tokens[0].RevokeReason)
	require.Equal(t, "explicit", *tokens[0].RevokeReason)

	_, err = env.core.ValidateBotToken(env.ctx, created.Secret)
	require.True(t, errors.Is(err, core.ErrBotTokenNotFound), "revoked token should not validate, got %v", err)
}
