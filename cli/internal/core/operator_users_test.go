package core

import (
	"testing"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestImportHistoricalUserKindsAndStatus(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	c := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "import-owner", "Owner", "")
	require.NoError(t, err)
	for _, tc := range []struct {
		name             string
		bot, deactivated bool
	}{
		{"human", false, false}, {"inactive-human", false, true},
		{"bot", true, false}, {"inactive-bot", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := HistoricalUserInput{Login: tc.name, DisplayName: tc.name, Deactivated: tc.deactivated}
			if tc.bot {
				input.BotOwnerID = owner.Id
			} else {
				input.Email = tc.name + "@example.test"
			}
			key := uuid.NewString()
			u, err := c.ImportHistoricalUser(ctx, key, input)
			require.NoError(t, err)
			require.Equal(t, tc.bot, u.IsBot)
			require.Equal(t, tc.deactivated, u.Deactivated)
			facts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(u.Id).AllEventsFilter())
			require.NoError(t, err)
			deactivations := 0
			for _, fact := range facts {
				require.Nil(t, fact.GetBotApiKeyCreated(), "imports must not issue bot credentials")
				require.Nil(t, fact.GetUserPasswordHashChanged(), "imports must not issue passwords")
				require.True(t, fact.GetImportContext().GetHiddenTimeline())
				if fact.GetUserAccountDeactivationChanged() != nil {
					deactivations++
				}
			}
			if tc.deactivated {
				require.Equal(t, 1, deactivations)
			} else {
				require.Zero(t, deactivations)
			}
			// A fresh replica reconstructs both account status and operation receipts.
			restarted := startReplica(t, ns, testCoreConfig())
			before, err := c.EventPublisher.LastStreamSeq(ctx)
			require.NoError(t, err)
			again, err := restarted.ImportHistoricalUser(ctx, key, input)
			require.NoError(t, err)
			require.Equal(t, u.Id, again.Id)
			require.Equal(t, tc.deactivated, again.Deactivated)
			after, err := c.EventPublisher.LastStreamSeq(ctx)
			require.NoError(t, err)
			require.Equal(t, before, after)
			input.Deactivated = !input.Deactivated
			_, err = restarted.ImportHistoricalUser(ctx, key, input)
			require.ErrorContains(t, err, "different arguments")
		})
	}
}

func TestImportHistoricalBotOwnerAndAccess(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	c := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "import-owner", "Owner", "")
	require.NoError(t, err)
	bot, err := c.ImportHistoricalUser(ctx, uuid.NewString(), HistoricalUserInput{Login: "inactive-bot", BotOwnerID: owner.Id, Deactivated: true})
	require.NoError(t, err)
	// Even credentials issued later must not bypass the imported status.
	key, err := c.CreateBotAPIKey(ctx, owner.Id, bot.Id, "test")
	require.NoError(t, err)
	_, err = c.ValidateBotAPIKey(ctx, key.Credential)
	require.ErrorIs(t, err, ErrAuthTokenNotFound)
	webhook, err := c.CreateBotIncomingWebhook(ctx, owner.Id, bot.Id, "test")
	require.NoError(t, err)
	_, err = c.ValidateBotIncomingWebhookCredential(ctx, webhook.Credential)
	require.ErrorIs(t, err, ErrAuthTokenNotFound)
	watch, cancel := c.WatchBotAPIKeyInvalidated(bot.Id, c.botAPIKeyVerifier(key.Credential))
	defer cancel()
	select {
	case <-watch:
	default:
		t.Fatal("inactive bot credential remained valid")
	}
	for _, input := range []HistoricalUserInput{
		{Login: "invalid-email", BotOwnerID: owner.Id, Email: "bot@example.test"},
		{Login: "missing-owner", BotOwnerID: "missing"},
		{Login: "bot-owner", BotOwnerID: bot.Id},
	} {
		_, err := c.ImportHistoricalUser(ctx, uuid.NewString(), input)
		require.Error(t, err)
	}
}
