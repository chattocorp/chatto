package core

import (
	"io"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestImportHistoricalAvatarRetryAndPreservation(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	c := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "avatar-owner", "Owner", "")
	require.NoError(t, err)
	bot, err := c.ImportHistoricalUser(ctx, uuid.NewString(), HistoricalUserInput{Login: "avatar-bot", BotOwnerID: owner.Id, Deactivated: true})
	require.NoError(t, err)
	data, err := io.ReadAll(createTestImage(30, 30))
	require.NoError(t, err)
	key := uuid.NewString()
	other := startReplica(t, ns, testCoreConfig())
	errors := make(chan error, 2)
	for _, replica := range []*ChattoCore{c, other} {
		go func() { errors <- replica.ImportHistoricalUserAvatar(ctx, key, bot.Id, data) }()
	}
	require.NoError(t, <-errors)
	require.NoError(t, <-errors)
	// A losing concurrent attempt must not append a second receipt when it
	// observes the winner's avatar and changes from assignment to preservation.
	receipts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.Aggregate{Type: "operator_operation", ID: key}.Subject("operator_operation_completed"))
	require.NoError(t, err)
	require.Len(t, receipts, 1)
	avatar, err := c.GetUserAvatar(ctx, bot.Id)
	require.NoError(t, err)
	require.NotNil(t, avatar)
	other = startReplica(t, ns, testCoreConfig())
	before, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	require.NoError(t, other.ImportHistoricalUserAvatar(ctx, key, bot.Id, data))
	after, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	// Another import cannot replace the current image.
	second := uuid.NewString()
	require.NoError(t, c.ImportHistoricalUserAvatar(ctx, second, bot.Id, []byte("not decoded when preserving")))
	preserved, err := c.GetUserAvatar(ctx, bot.Id)
	require.NoError(t, err)
	require.Equal(t, avatar.Id, preserved.Id)
	_, err = c.ClearUserAvatar(ctx, owner.Id, bot.Id)
	require.NoError(t, err)
	require.NoError(t, other.ImportHistoricalUserAvatar(ctx, key, bot.Id, data))
	require.NoError(t, other.ImportHistoricalUserAvatar(ctx, second, bot.Id, []byte("not decoded when preserving")))
	empty, err := c.GetUserAvatar(ctx, bot.Id)
	require.NoError(t, err)
	require.Nil(t, empty)
	require.ErrorContains(t, c.ImportHistoricalUserAvatar(ctx, key, owner.Id, data), "different arguments")
	require.Error(t, c.ImportHistoricalUserAvatar(ctx, uuid.NewString(), owner.Id, []byte("invalid image")))
}
