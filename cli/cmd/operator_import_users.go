package cmd

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	op "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
)

type importUserMatch struct {
	leader  int
	account string
	emails  []string
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
				if user.GetDeleted() || user.GetBot() != nil {
					return fmt.Errorf("users.jsonl record %d: matched account is not an active human", i+1)
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
