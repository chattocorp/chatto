package interchange

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
)

type messageState struct {
	room, author, thread, reply string
	created                     *string
	final, deleted              bool
	exposed                     bool
	reactions                   map[string]bool
}

func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// validateReferences enforces causal references and final-state semantics after
// structural validation. Errors disclose positions and rule names only.
func (a *Archive) validateReferences() error {
	fail := func(code string) error { return fmt.Errorf("import: %s", code) }
	parsed, err := uuid.Parse(a.Manifest.ExportID)
	if err != nil || !strings.EqualFold(parsed.String(), a.Manifest.ExportID) {
		return fail("invalid export identity")
	}
	partial := a.Manifest.History == "partial"
	omissions := map[string]bool{}
	for _, o := range a.Manifest.Omissions {
		omissions[o.Category] = true
	}
	if partial && !omissions["history"] {
		return fail("partial history requires an omission")
	}
	var cut time.Time
	if a.Manifest.CapturedAt != nil {
		cut, err = time.Parse(time.RFC3339Nano, *a.Manifest.CapturedAt)
		if err != nil {
			return fail("invalid capture time")
		}
	}
	if (a.Manifest.CapturedAt == nil && a.Manifest.Consistency != "unknown") || (a.Manifest.Consistency == "unknown" && !partial) {
		return fail("invalid capture consistency")
	}
	checkTime := func(s *string) error {
		if s == nil {
			return nil
		}
		t, err := time.Parse(time.RFC3339Nano, *s)
		if err != nil || (a.Manifest.CapturedAt != nil && t.After(cut)) {
			return fail("invalid historical time")
		}
		return nil
	}
	users := map[string]bool{}
	rooms := map[string]Room{}
	assets := map[string]bool{}
	paths := map[string]bool{}
	for _, u := range a.Users {
		if users[u.ID] {
			return fail("duplicate user ID")
		}
		users[u.ID] = true
		for _, email := range u.Emails {
			address, err := mail.ParseAddress(email)
			if err != nil || address.Address != email {
				return fail("invalid source email")
			}
		}
	}
	for _, r := range a.Rooms {
		if _, ok := rooms[r.ID]; ok {
			return fail("duplicate room ID")
		}
		rooms[r.ID] = r
	}
	for _, asset := range a.Assets {
		if assets[asset.ID] {
			return fail("duplicate asset ID")
		}
		assets[asset.ID] = true
		if asset.Storage.Status != "available" {
			if !omissions["files"] {
				return fail("missing file omission")
			}
			continue
		}
		if paths[asset.Storage.Path] {
			return fail("duplicate asset path")
		}
		paths[asset.Storage.Path] = true
		digest, ok := a.sums[asset.Storage.Path]
		if !ok {
			return fail("missing asset file")
		}
		if a.sizes[asset.Storage.Path] != asset.Storage.Size || digest != asset.Storage.SHA256 {
			return fail("asset integrity mismatch")
		}
	}
	if len(paths)+len(names) != len(a.sums) {
		return fail("unlisted asset file")
	}
	for _, u := range a.Users {
		if u.AvatarAssetID != "" && !assets[u.AvatarAssetID] {
			return fail("unknown user avatar asset")
		}
	}
	checkContent := func(c Content) error {
		if c.Status == "available" {
			if strings.TrimSpace(c.Body.Text) == "" && len(c.AssetIDs) == 0 {
				return fail("empty content")
			}
			for _, id := range c.AssetIDs {
				if !assets[id] {
					return fail("unknown asset reference")
				}
			}
		} else if c.Status == "unavailable" && !omissions["content"] {
			return fail("missing content omission")
		}
		return nil
	}
	messages := map[string]*messageState{}
	threads := map[string]bool{}
	ids := map[string]bool{}
	members := map[string]map[string]bool{}
	finalMembers := map[string]bool{}
	metadata := map[string]map[string]string{}
	a.EventCount = 0
	err = a.VisitEvents(func(e Event) error {
		a.EventCount++
		err := func() error {
			if ids[e.ID] {
				return fail("duplicate event ID")
			}
			ids[e.ID] = true
			if _, ok := rooms[e.RoomID]; !ok {
				return fail("unknown room reference")
			}
			if e.ActorID != nil && !users[*e.ActorID] {
				return fail("unknown actor reference")
			}
			if err := checkTime(e.OccurredAt); err != nil {
				return err
			}
			d := e.Data
			switch e.Type {
			case "message.posted", "message.snapshot":
				final := e.Type == "message.snapshot"
				old := messages[d.MessageID]
				if old != nil && (!final || old.final) {
					return fail("duplicate message introduction or snapshot")
				}
				if final && !partial {
					return fail("message snapshot requires partial history")
				}
				if !users[d.AuthorID] {
					return fail("unknown author reference")
				}
				for _, target := range []*string{d.ThreadID, d.ReplyTo} {
					if target != nil {
						m := messages[*target]
						if m == nil || m.room != e.RoomID {
							return fail("invalid message reference")
						}
					}
				}
				if d.ThreadID != nil {
					if messages[*d.ThreadID].thread != "" {
						return fail("nested thread")
					}
					threads[*d.ThreadID] = true
				}
				if err := checkContent(d.Content); err != nil {
					return err
				}
				if !final && d.Content.Status == "deleted" {
					return fail("post cannot contain a tombstone")
				}
				created := e.OccurredAt
				if final {
					created = d.CreatedAt
				}
				m := &messageState{room: e.RoomID, author: d.AuthorID, thread: value(d.ThreadID), reply: value(d.ReplyTo), created: created, final: final, deleted: d.Content.Status == "deleted", reactions: map[string]bool{}}
				if old != nil {
					if m.room != old.room || m.author != old.author || m.thread != old.thread || m.reply != old.reply {
						return fail("snapshot changes message identity")
					}
					if created == nil || !sameTime(created, old.created) {
						return fail("snapshot changes creation time")
					}
					if old.deleted && !m.deleted {
						return fail("snapshot resurrects deleted content")
					}
					m.exposed = old.exposed
				}
				if final {
					for _, t := range []*string{d.CreatedAt, d.EditedAt, d.DeletedAt} {
						if err := checkTime(t); err != nil {
							return err
						}
					}
					for _, t := range []*string{d.EditedAt, d.DeletedAt} {
						if before(t, created) {
							return fail("mutation precedes creation")
						}
					}
					if before(d.DeletedAt, d.EditedAt) {
						return fail("edit follows deletion")
					}
					if !m.deleted && d.DeletedAt != nil {
						return fail("deletion time on nondeleted content")
					}
					emojis := map[string]bool{}
					for _, reaction := range d.Reactions {
						if emojis[reaction.Emoji] {
							return fail("duplicate reaction group")
						}
						emojis[reaction.Emoji] = true
						for _, id := range reaction.UserIDs {
							if !users[id] {
								return fail("unknown reactor reference")
							}
							m.reactions[reaction.Emoji+"\x00"+id] = true
						}
					}
				}
				if !m.deleted {
					m.exposed = m.exposed || d.Content.Status != "unavailable"
				}
				messages[d.MessageID] = m
			case "message.edited", "message.deleted", "reaction.added", "reaction.removed":
				m := messages[d.MessageID]
				if m == nil || m.room != e.RoomID {
					return fail("invalid message reference")
				}
				if m.final {
					return fail("mutation follows final message snapshot")
				}
				if (e.Type == "message.edited" || e.Type == "message.deleted") && before(e.OccurredAt, m.created) {
					return fail("mutation precedes creation")
				}
				if e.Type == "message.edited" {
					if m.deleted || d.Content.Status == "deleted" {
						return fail("invalid edit after deletion")
					}
					if err := checkContent(d.Content); err != nil {
						return err
					}
					m.exposed = m.exposed || d.Content.Status != "unavailable"
				} else if e.Type == "message.deleted" {
					if m.deleted {
						return fail("duplicate deletion")
					}
					m.deleted = true
				} else {
					if !users[d.UserID] {
						return fail("unknown reactor reference")
					}
					key := d.Emoji + "\x00" + d.UserID
					want := e.Type == "reaction.added"
					old, known := m.reactions[key]
					if (known || !partial) && old == want {
						return fail("invalid reaction transition")
					}
					m.reactions[key] = want
				}
			case "thread.created":
				m := messages[d.RootID]
				if m == nil || m.room != e.RoomID || m.thread != "" || threads[d.RootID] {
					return fail("invalid thread declaration")
				}
				threads[d.RootID] = true
			case "room.member_joined", "room.member_left", "room.membership_snapshot":
				if finalMembers[e.RoomID] {
					return fail("membership follows final snapshot")
				}
				if members[e.RoomID] == nil {
					members[e.RoomID] = map[string]bool{}
				}
				state := members[e.RoomID]
				if e.Type == "room.membership_snapshot" {
					final := map[string]bool{}
					for _, id := range d.UserIDs {
						if !users[id] {
							return fail("unknown member reference")
						}
						final[id] = true
					}
					if !partial {
						for id, yes := range state {
							if final[id] != yes {
								return fail("final membership differs from complete history")
							}
						}
						for id := range final {
							if !state[id] {
								return fail("final membership differs from complete history")
							}
						}
					}
					finalMembers[e.RoomID] = true
				} else {
					if !users[d.UserID] {
						return fail("unknown member reference")
					}
					want := e.Type == "room.member_joined"
					old, known := state[d.UserID]
					if (known || !partial) && old == want {
						return fail("invalid membership transition")
					}
					state[d.UserID] = want
					emojis := map[string]bool{}
					for _, reaction := range d.Reactions {
						if emojis[reaction.Emoji] {
							return fail("duplicate reaction group")
						}
						emojis[reaction.Emoji] = true
						for _, id := range reaction.UserIDs {
							if !users[id] {
								return fail("unknown reactor reference")
							}
						}
					}
				}
			default:
				if metadata[e.RoomID] == nil {
					metadata[e.RoomID] = map[string]string{}
				}
				switch e.Type {
				case "room.name_changed":
					metadata[e.RoomID]["name"] = d.Value
				case "room.topic_changed":
					metadata[e.RoomID]["topic"] = d.Value
				case "room.purpose_changed":
					metadata[e.RoomID]["purpose"] = d.Value
				case "room.archived":
					metadata[e.RoomID]["archived"] = "true"
				case "room.unarchived":
					metadata[e.RoomID]["archived"] = "false"
				default:
					return errors.New("import: unsupported event")
				}
			}
			return nil
		}()
		return err
	})
	if err != nil {
		return err
	}
	if len(finalMembers) != len(rooms) {
		return fail("missing final membership snapshot")
	}
	if !partial {
		for id, fields := range metadata {
			r := rooms[id]
			want := map[string]string{"name": r.Name, "topic": r.Topic, "purpose": r.Purpose, "archived": fmt.Sprint(r.Archived)}
			for field, v := range fields {
				if want[field] != v {
					return fail("final room metadata differs from complete history")
				}
			}
		}
	}
	for _, m := range messages {
		if m.deleted && m.exposed {
			return fail("deleted content exposed in earlier history")
		}
	}
	return nil
}

func before(a, b *string) bool {
	if a == nil || b == nil {
		return false
	}
	ta, e1 := time.Parse(time.RFC3339Nano, *a)
	tb, e2 := time.Parse(time.RFC3339Nano, *b)
	return e1 == nil && e2 == nil && ta.Before(tb)
}
func sameTime(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	ta, e1 := time.Parse(time.RFC3339Nano, *a)
	tb, e2 := time.Parse(time.RFC3339Nano, *b)
	return e1 == nil && e2 == nil && ta.Equal(tb)
}
