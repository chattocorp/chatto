package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

type importUserMatch struct {
	leader  int
	account string
	emails  []string
}

// matchUsers resolves connected email identities before writes. A component
// can reuse one existing account, but must never merge two existing accounts.
// The first source record supplies the profile when a new account is needed.
func (m *archiveImport) matchUsers(ctx context.Context) error {
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
			if owner, ok := m.core.userModel.emailOwnerID(email); ok {
				if old := owners[i]; old != "" && old != owner {
					return invalidArgument(fmt.Sprintf("users.jsonl record %d: emails belong to different accounts", i+1))
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
		if i+1 < len(m.receipts) {
			owners[i] = m.receipts[i+1].ResourceId
		}
	}
	m.userMatches = make([]importUserMatch, n)
	emails := map[int]map[string]bool{}
	for i, u := range m.archive.Users {
		leader := root(i)
		p := &m.userMatches[leader]
		if owner := owners[i]; owner != "" {
			if p.account != "" && p.account != owner {
				return invalidArgument("source emails would merge existing accounts")
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
			if id := m.userMatches[i].account; id != "" {
				if err := m.core.requireHumanUser(ctx, id); err != nil {
					return err
				}
				// Encryption must not lazily append a key outside the import
				// receipt and invalidate the unfinished import's concurrency fence.
				if err := m.core.userModel.waitForContentKeysCurrent(ctx, id); err != nil {
					return err
				}
				for _, purpose := range []evtv1.UserDEKPurpose{evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, evtv1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY} {
					_, ok, err := m.core.userModel.activeContentKey(id, purpose)
					if err != nil {
						return err
					}
					if !ok {
						return invalidArgument("matched account requires content key initialization before import")
					}
				}
			}
		}
	}
	for i := range m.userMatches {
		m.userMatches[i] = m.userMatches[root(i)]
	}
	return nil
}

// matchedUserEntries adds only unowned email claims. The caller commits these
// facts and its receipt under the whole-EVT guard captured before matching.
func (m *archiveImport) matchedUserEntries(ctx context.Context, account string, emails []string) ([]evtstream.BatchEntry, error) {
	var entries []evtstream.BatchEntry
	for _, email := range emails {
		if owner, ok := m.core.userModel.emailOwnerID(email); ok {
			if owner != account {
				return nil, ErrEmailAlreadyVerified
			}
			continue
		}
		e := newEvent(SystemActorID, &evtv1.Event{ImportContext: m.context(true, true), Event: &evtv1.Event_UserVerifiedEmailAdded{UserVerifiedEmailAdded: &evtv1.UserVerifiedEmailAddedEvent{UserId: account}}})
		encrypted, err := m.core.encryptUserPIIString(ctx, e.Id, account, evtstream.EventUserVerifiedEmailAdded, "email", email)
		if err != nil {
			return nil, err
		}
		e.GetUserVerifiedEmailAdded().EncryptedEmail = encrypted
		entries = append(entries, evtstream.BatchEntry{Subject: evtstream.UserAggregate(account).SubjectFor(e), Event: e})
	}
	return entries, nil
}
