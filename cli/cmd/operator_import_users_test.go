package cmd

import (
	"path/filepath"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	"github.com/stretchr/testify/require"
)

func TestOperatorImportBotsAndDeactivatedUsers(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "import-owner", "Owner", "")
	require.NoError(t, err)
	require.NoError(t, env.core.AssignServerRole(env.ctx, core.SystemActorID, owner.Id, core.RoleOwner))
	a, err := interchange.ReadDirectory("../../schema/interchange/v1/examples/conversation")
	require.NoError(t, err)
	defer a.Close()
	a.Users[0].Status = "deactivated"
	a.Users[1].Kind = "bot"
	a.Users[1].Status = "deactivated"
	a.Users[1].Username = nil
	m, err := newCLIArchiveImport(a, importNaming{}, filepath.Join(t.TempDir(), "journal"))
	require.NoError(t, err)
	defer m.close()
	require.NoError(t, m.run(env.ctx, false, true))
	require.Equal(t, owner.Id, m.progress.values["bot-owner"])
	for _, source := range a.Users {
		u, err := env.core.GetUser(env.ctx, m.users[source.ID])
		require.NoError(t, err)
		require.True(t, u.Deactivated)
		require.Equal(t, source.Kind == "bot", u.IsBot)
		if u.IsBot {
			require.Equal(t, owner.Id, u.BotOwnerUserId)
		}
	}
	secondOwner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "second-owner", "Second owner", "")
	require.NoError(t, err)
	require.NoError(t, env.core.AssignServerRole(env.ctx, core.SystemActorID, secondOwner.Id, core.RoleOwner))
	before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	// Lose successful account results, retaining pending markers and operation keys.
	delete(m.progress.values, "complete")
	for _, source := range a.Users {
		delete(m.progress.values, "user/"+importRef(source.ID))
	}
	m.users, m.rooms, m.messages, m.authors = map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}
	m.botOwnerID = ""
	require.NoError(t, m.run(env.ctx, false, true))
	after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestOperatorImportMatchedAccountKeepsStatus(t *testing.T) {
	for _, active := range []bool{true, false} {
		t.Run(map[bool]string{true: "active", false: "deactivated"}[active], func(t *testing.T) {
			env := newOperatorCLITestEnv(t)
			existing, err := env.core.ImportHistoricalUser(env.ctx, "ab86d13d-3ed9-4a75-943e-1b0b357dc5a3", core.HistoricalUserInput{Login: "existing", Email: "alice@example.test", Deactivated: !active})
			require.NoError(t, err)
			a, err := interchange.ReadDirectory("../../schema/interchange/v1/examples/conversation")
			require.NoError(t, err)
			defer a.Close()
			a.Users[0].Emails = []string{"ALICE@example.test"}
			if active {
				a.Users[0].Status = "deactivated"
			}
			m, err := newCLIArchiveImport(a, importNaming{}, filepath.Join(t.TempDir(), "journal"))
			require.NoError(t, err)
			defer m.close()
			require.NoError(t, m.run(env.ctx, false, true))
			require.Equal(t, existing.Id, m.users[a.Users[0].ID])
			u, err := env.core.GetUser(env.ctx, existing.Id)
			require.NoError(t, err)
			require.Equal(t, !active, u.Deactivated)
			require.Equal(t, "existing", u.Login)
		})
	}
}

func TestOperatorImportBotOwnerRequiredBeforeWrites(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	a, err := interchange.ReadDirectory("../../schema/interchange/v1/examples/conversation")
	require.NoError(t, err)
	defer a.Close()
	a.Users[1].Kind = "bot"
	m, err := newCLIArchiveImport(a, importNaming{}, filepath.Join(t.TempDir(), "journal"))
	require.NoError(t, err)
	defer m.close()
	before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.ErrorContains(t, m.run(env.ctx, false, true), "--bot-owner")
	after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestOperatorImportAccountConflictsBeforeWrites(t *testing.T) {
	for _, scenario := range []string{"bot-email", "shared-status", "deleted", "inactive-owner", "multiple-owners"} {
		t.Run(scenario, func(t *testing.T) {
			env := newOperatorCLITestEnv(t)
			a, err := interchange.ReadDirectory("../../schema/interchange/v1/examples/conversation")
			require.NoError(t, err)
			defer a.Close()
			naming := importNaming{}
			expected := ""
			switch scenario {
			case "bot-email":
				a.Users[1].Kind, a.Users[1].Emails = "bot", []string{"bot@example.test"}
				expected = "cannot have emails"
			case "shared-status":
				a.Users[0].Emails, a.Users[1].Emails = []string{"shared@example.test"}, []string{"shared@example.test"}
				a.Users[1].Status = "deactivated"
				expected = "conflicting account kinds or statuses"
			case "deleted":
				a.Users[1].Status = "deleted"
				expected = "unsupported account kind or status"
			case "inactive-owner":
				_, err = env.core.ImportHistoricalUser(env.ctx, "c063e4ec-5191-4327-95ea-e86bd53cb9f3", core.HistoricalUserInput{Login: "inactive-owner", Deactivated: true})
				require.NoError(t, err)
				a.Users[1].Kind = "bot"
				naming.BotOwner = "inactive-owner"
				expected = "bot owner must be an active human"
			case "multiple-owners":
				for _, login := range []string{"first-owner", "second-owner"} {
					owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, login, login, "")
					require.NoError(t, err)
					require.NoError(t, env.core.AssignServerRole(env.ctx, core.SystemActorID, owner.Id, core.RoleOwner))
				}
				a.Users[1].Kind = "bot"
				expected = "multiple active server owners"
			}
			m, err := newCLIArchiveImport(a, naming, filepath.Join(t.TempDir(), "journal"))
			require.NoError(t, err)
			defer m.close()
			before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
			require.NoError(t, err)
			require.ErrorContains(t, m.run(env.ctx, false, true), expected)
			after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
