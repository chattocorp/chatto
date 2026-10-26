package core

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	"github.com/stretchr/testify/require"
)

func archiveWithEmails(t *testing.T, emails ...[]string) *interchange.Archive {
	t.Helper()
	a, err := interchange.ReadDirectory("../../../schema/interchange/v1/examples/conversation")
	require.NoError(t, err)
	var output bytes.Buffer
	for i, line := range bytes.Split(bytes.TrimSpace(a.Metadata["users.jsonl"]), []byte{'\n'}) {
		var user map[string]any
		require.NoError(t, json.Unmarshal(line, &user))
		user["emails"] = emails[i]
		require.NoError(t, json.NewEncoder(&output).Encode(user))
	}
	a.Metadata["users.jsonl"] = output.Bytes()
	return a
}

func TestImportArchiveMatchesExistingEmailsAndCreatesUnownedEmails(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	existing, err := c.CreateVerifiedUser(ctx, SystemActorID, "existing", "Existing profile", "password123", "alice@example.com")
	require.NoError(t, err)
	a := archiveWithEmails(t, []string{"ALICE@EXAMPLE.COM", "alias@example.com"}, []string{"bob@example.com"})
	naming := ImportNaming{UsernamePrefix: "old-"}
	_, err = c.ImportArchive(ctx, a, naming, false, true)
	require.NoError(t, err)
	matched, err := c.GetUserByVerifiedEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	require.Equal(t, existing.Id, matched.Id)
	require.Equal(t, "existing", matched.Login)
	require.Equal(t, "Existing profile", matched.DisplayName)
	alias, err := c.GetUserByVerifiedEmail(ctx, "alias@example.com")
	require.NoError(t, err)
	require.Equal(t, existing.Id, alias.Id)
	created, err := c.GetUserByVerifiedEmail(ctx, "bob@example.com")
	require.NoError(t, err)
	require.Equal(t, "old-bob", created.Login)
	users, err := c.ListUsers(ctx)
	require.NoError(t, err)
	require.Len(t, users, 2)
	progress, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.ImportAggregate().AllEventsFilter())
	require.NoError(t, err)
	require.Equal(t, existing.Id, progress[1].GetImportProgressed().ResourceId)
	before, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	_, err = c.ImportArchive(ctx, a, naming, false, true)
	require.NoError(t, err)
	after, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestImportArchiveCoalescesSourceEmailAliases(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	a := archiveWithEmails(t, []string{"shared@example.com"}, []string{"SHARED@example.com", "alias@example.com"})
	_, err := c.ImportArchive(ctx, a, ImportNaming{}, false, true)
	require.NoError(t, err)
	users, err := c.ListUsers(ctx)
	require.NoError(t, err)
	require.Len(t, users, 1)
	alias, err := c.GetUserByVerifiedEmail(ctx, "alias@example.com")
	require.NoError(t, err)
	require.Equal(t, users[0].Id, alias.Id)
	rooms, err := c.ListRooms(ctx, KindChannel)
	require.NoError(t, err)
	members, err := c.GetRoomMembersList(ctx, KindChannel, rooms[0].Id)
	require.NoError(t, err)
	require.Len(t, members, 1)
}

func TestImportArchiveRejectsConflictingEmailOwnersBeforeWrites(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	_, err := c.CreateVerifiedUser(ctx, SystemActorID, "first", "First", "", "first@example.com")
	require.NoError(t, err)
	_, err = c.CreateVerifiedUser(ctx, SystemActorID, "second", "Second", "", "second@example.com")
	require.NoError(t, err)
	a := archiveWithEmails(t, []string{"first@example.com", "bridge@example.com"}, []string{"bridge@example.com", "second@example.com"})
	before, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	_, err = c.ImportArchive(ctx, a, ImportNaming{}, false, true)
	require.ErrorContains(t, err, "merge existing accounts")
	after, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
