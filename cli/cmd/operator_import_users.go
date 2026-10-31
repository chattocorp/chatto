package cmd

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	op "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
)

type importUserMatch struct {
	leader  int
	account string
	emails  []string
}

// resolveBotOwner uses the current server owner by default. Persisting the
// selected ID before account creation keeps retries independent of later role
// or username changes. Multiple owners require an explicit choice.
func (m *cliArchiveImport) resolveBotOwner(ctx context.Context) error {
	lookup := &op.GetUserRequest{}
	if id := m.progress.values["bot-owner"]; id != "" {
		lookup.UserId = id
	} else if m.naming.BotOwner != "" {
		lookup.Login = m.naming.BotOwner
	} else {
		for offset := int32(0); ; {
			res, err := m.userAPI.ListUsers(ctx, operatorRequest(&op.ListUsersRequest{Page: &apiv1.PageRequest{Limit: 100, Offset: offset}}))
			if err != nil {
				return fmt.Errorf("resolve server owner: %w", err)
			}
			for _, member := range res.Msg.GetUsers() {
				u := member.GetUser()
				if slices.Contains(member.GetRoles(), core.RoleOwner) && u.GetId() != "" && !u.GetDeleted() && !u.GetDeactivated() && u.GetBot() == nil {
					if lookup.UserId != "" && lookup.UserId != u.GetId() {
						return fmt.Errorf("multiple active server owners; select one with --bot-owner")
					}
					lookup.UserId = u.GetId()
				}
			}
			if !res.Msg.GetPage().GetHasMore() {
				break
			}
			if len(res.Msg.GetUsers()) == 0 {
				return fmt.Errorf("server returned an empty owner lookup page with more results")
			}
			offset += int32(len(res.Msg.GetUsers()))
		}
		if lookup.UserId == "" {
			return fmt.Errorf("no active server owner; configure one or select an active human with --bot-owner")
		}
	}
	res, err := m.userAPI.GetUser(ctx, operatorRequest(lookup))
	if err != nil {
		return fmt.Errorf("resolve bot owner: %w", err)
	}
	u := res.Msg.GetMember().GetUser()
	if u.GetId() == "" || u.GetDeleted() || u.GetDeactivated() || u.GetBot() != nil {
		return fmt.Errorf("bot owner must be an active human")
	}
	m.botOwnerID = u.GetId()
	return nil
}

// importUsername retains supplied names and gives nameless integrations a stable
// destination name. Normal collision checks apply to generated names too.
func (m *cliArchiveImport) importUsername(u interchange.User) string {
	if u.Username != nil {
		return m.username(*u.Username)
	}
	return m.username("bot-" + importRef(u.ID)[:24])
}

// matchUsers resolves connected email identities before writes. A component
// can reuse one existing account, but must never merge two existing accounts.
// The first source record supplies the profile when a new account is needed.
func (m *cliArchiveImport) matchUsers(ctx context.Context) error {
	n := len(m.archive.Users)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		if parent[i] != i {
			parent[i] = root(parent[i])
		}
		return parent[i]
	}
	join := func(a, b int) {
		a, b = root(a), root(b)
		if a > b {
			a, b = b, a
		}
		parent[b] = a
	}
	seen := map[string]int{}
	owners := map[int]string{}
	for i, u := range m.archive.Users {
		for _, raw := range u.Emails {
			email := strings.ToLower(strings.TrimSpace(raw))
			key := "email:" + email
			if previous, ok := seen[key]; ok {
				join(i, previous)
			} else {
				seen[key] = i
			}
			response, err := m.userAPI.GetUser(ctx, operatorRequest(&op.GetUserRequest{Email: email}))
			if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
				return err
			}
			if err == nil {
				user := response.Msg.GetMember().GetUser()
				if user.GetDeleted() || user.GetBot() != nil || u.Kind != "human" {
					return fmt.Errorf("users.jsonl record %d: email matching requires human accounts", i+1)
				}
				owner := response.Msg.GetMember().GetUser().GetId()
				if old := owners[i]; old != "" && old != owner {
					return fmt.Errorf("users.jsonl record %d: emails belong to different accounts", i+1)
				}
				owners[i] = owner
				key = "account:" + owner
				if previous, ok := seen[key]; ok {
					join(i, previous)
				} else {
					seen[key] = i
				}
			}
		}
		if id := m.progress.values["user/"+importRef(u.ID)]; id != "" {
			if owner := owners[i]; owner != "" && owner != id {
				return fmt.Errorf("users.jsonl record %d: email ownership changed since import", i+1)
			}
			owners[i] = id
		}
	}
	m.userMatches = make([]importUserMatch, n)
	emails := map[int]map[string]bool{}
	for i, u := range m.archive.Users {
		leader := root(i)
		if u.Kind != m.archive.Users[leader].Kind || u.Status != m.archive.Users[leader].Status {
			return fmt.Errorf("users.jsonl record %d: shared email identities have conflicting account kinds or statuses", i+1)
		}
		p := &m.userMatches[leader]
		if owner := owners[i]; owner != "" {
			if p.account != "" && p.account != owner {
				return fmt.Errorf("source emails would merge existing accounts")
			}
			p.account = owner
		}
		if emails[leader] == nil {
			emails[leader] = map[string]bool{}
		}
		for _, email := range u.Emails {
			emails[leader][strings.ToLower(strings.TrimSpace(email))] = true
		}
	}
	for i := range m.userMatches {
		if root(i) == i {
			m.userMatches[i].leader = i
			m.userMatches[i].emails = sortedImportKeys(emails[i])
		}
	}
	for i := range m.userMatches {
		m.userMatches[i] = m.userMatches[root(i)]
	}
	return nil
}
