package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chattocorp/chatto/cli/internal/assets"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ImportNaming decorates destination names without changing source references.
// These options are immutable for a resumed import.
type ImportNaming struct {
	ChannelPrefix  string
	ChannelSuffix  string
	UsernamePrefix string
	UsernameSuffix string
}

// ImportArchiveResult reports source record counts, not newly created accounts,
// without exposing source personal data.
type ImportArchiveResult struct {
	Users, Rooms, Assets, Events, Omissions int
	Complete                                bool
}

type archiveImport struct {
	core          *ChattoCore
	archive       *interchange.Archive
	naming        ImportNaming
	fingerprint   string
	seq           uint64
	receipts      []*evtv1.ImportProgressedEvent
	users, rooms  map[string]string
	assetBindings map[string]importAssetBinding
	userMatches   []importUserMatch
}

type importAssetBinding struct{ author, room, message string }

func (m *archiveImport) channel(name string) string {
	return m.naming.ChannelPrefix + name + m.naming.ChannelSuffix
}
func (m *archiveImport) username(name string) string {
	return m.naming.UsernamePrefix + name + m.naming.UsernameSuffix
}

// nativeID is stable across retries and does not expose the source identifier.
// Different resource namespaces cannot collide by reusing a source ID.
func (m *archiveImport) nativeID(kind, id string) string {
	// The start receipt supplies a random destination namespace. Identical
	// archives imported on different servers must not reuse asset storage keys.
	h := sha256.Sum256([]byte(m.receipts[0].ResourceId + "\x00" + kind + "\x00" + id))
	return base64.RawURLEncoding.EncodeToString(h[:])[:21]
}

func (m *archiveImport) context(hidden, unknown bool) *evtv1.ImportContext {
	return &evtv1.ImportContext{ExportId: m.archive.Manifest.ExportID, HiddenTimeline: hidden, UnknownTime: unknown}
}

func (m *archiveImport) load(ctx context.Context) error {
	facts, seq, err := m.core.EventPublisher.SubjectEvents(ctx, evtstream.ImportAggregate().AllEventsFilter())
	if err != nil {
		return err
	}
	m.receipts = nil
	m.seq = seq
	for i, fact := range facts {
		r := fact.GetImportProgressed()
		if r == nil || r.GetOperationIndex() != uint64(i) || r.GetFingerprint() != m.fingerprint || (i == 0 && r.GetResourceId() == "") {
			return invalidArgument("destination belongs to another archive or different import options")
		}
		m.receipts = append(m.receipts, r)
	}
	return nil
}

func (m *archiveImport) receipt(index int, resource string, complete bool) evtstream.BatchEntry {
	e := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_ImportProgressed{ImportProgressed: &evtv1.ImportProgressedEvent{
		Fingerprint: m.fingerprint, OperationIndex: uint64(index), ResourceId: resource, Complete: complete,
	}}})
	return evtstream.BatchEntry{Subject: evtstream.ImportAggregate().SubjectFor(e), Event: e, Expect: events.ExpectFilterSeq(evtstream.EventSubjectFilter(), m.seq)}
}

func (m *archiveImport) commit(ctx context.Context, index int, resource string, complete bool, entries []evtstream.BatchEntry) error {
	if index < len(m.receipts) {
		return nil
	}
	if index != len(m.receipts) {
		return fmt.Errorf("import operation order mismatch")
	}
	entries = append(entries, m.receipt(index, resource, complete))
	seqs, err := m.core.EventPublisher.AppendBatch(ctx, entries)
	if err != nil {
		return fmt.Errorf("import interrupted; retry the same archive and options: %w", err)
	}
	m.seq = seqs[len(seqs)-1]
	m.receipts = append(m.receipts, entries[len(entries)-1].Event.GetImportProgressed())
	return nil
}

// ImportArchive is a root-equivalent operator operation. It validates the entire
// bounded archive before writes. Each operation and its receipt commit together.
// The first version requires exclusive use of a destination without history until complete;
// unrelated EVT writes fence continuation rather than overwrite newer activity.
func (c *ChattoCore) ImportArchive(ctx context.Context, archive *interchange.Archive, naming ImportNaming, validateOnly, acceptOmissions bool) (*ImportArchiveResult, error) {
	if archive == nil {
		return nil, invalidArgument("archive is required")
	}
	if err := archive.Validate(); err != nil {
		return nil, invalidArgument(err.Error())
	}
	options, _ := json.Marshal(naming)
	digest := sha256.Sum256(append([]byte(archive.Digest()), options...))
	m := &archiveImport{core: c, archive: archive, naming: naming, fingerprint: hex.EncodeToString(digest[:]), users: map[string]string{}, rooms: map[string]string{}}
	// Capture the fence before reading receipts. Otherwise a second importer
	// could see an empty journal, then capture the first importer's start as
	// its fence and incorrectly append a second operation-zero receipt.
	tail, err := c.EventPublisher.LastStreamSeq(ctx)
	if err != nil {
		return nil, err
	}
	if err := m.load(ctx); err != nil {
		return nil, err
	}
	result := &ImportArchiveResult{Users: len(archive.Users), Rooms: len(archive.Rooms), Assets: len(archive.Assets), Events: len(archive.Events), Omissions: len(archive.Manifest.Omissions)}
	if len(m.receipts) > 0 && m.receipts[len(m.receipts)-1].Complete {
		if err := m.waitForViews(ctx); err != nil {
			return nil, err
		}
		result.Complete = true
		return result, nil
	}
	if len(m.receipts) > 0 && tail != m.seq {
		return nil, invalidArgument("destination changed outside the unfinished import")
	}
	if err := m.preflight(ctx); err != nil {
		return nil, err
	}
	if validateOnly {
		return result, nil
	}
	if len(archive.Manifest.Omissions) > 0 && !acceptOmissions {
		return nil, invalidArgument("archive declares omissions; pass --accept-omissions to import it")
	}
	if len(m.receipts) == 0 {
		m.seq = tail
		if err := m.commit(ctx, 0, NewEventID(), false, nil); err != nil {
			return nil, err
		}
	}
	index := 1
	for sourceIndex, user := range archive.Users {
		if index >= len(m.receipts) {
			match := m.userMatches[sourceIndex]
			account := match.account
			if match.leader < sourceIndex {
				account = m.receipts[match.leader+1].ResourceId
			}
			if account != "" {
				var entries []evtstream.BatchEntry
				if match.leader == sourceIndex {
					entries, err = m.matchedUserEntries(ctx, account, match.emails)
					if err != nil {
						return nil, err
					}
				}
				if err := m.commit(ctx, index, account, false, entries); err != nil {
					return nil, err
				}
			} else {
				_, err := c.createUserWithOptions(ctx, SystemActorID, m.username(*user.Username), user.DisplayName, "", userCreationOptions{
					importContext: m.context(true, true),
					importEmails:  match.emails,
					importReceipt: func(id string) evtstream.BatchEntry { return m.receipt(index, id, false) },
				})
				if err != nil {
					return nil, fmt.Errorf("import user record %d: %w", index, err)
				}
				if err := m.load(ctx); err != nil {
					return nil, err
				}
			}
		}
		m.users[user.ID] = m.receipts[index].ResourceId
		index++
	}
	for _, room := range archive.Rooms {
		if index >= len(m.receipts) {
			_, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", m.channel(room.Name), room.Topic, func(o *createRoomOptions) {
				o.importContext = m.context(true, true)
				o.importPurpose = room.Purpose
				o.importReceipt = func(id string) evtstream.BatchEntry { return m.receipt(index, id, false) }
			})
			if err != nil {
				return nil, fmt.Errorf("import room record %d: %w", index, err)
			}
			if err := m.load(ctx); err != nil {
				return nil, err
			}
		}
		m.rooms[room.ID] = m.receipts[index].ResourceId
		index++
	}
	for _, asset := range archive.Assets {
		if index >= len(m.receipts) {
			owner, roomID, messageID := m.assetOwner(asset.ID)
			data := archive.Files[asset.Storage.Path]
			var image assets.ImageInfo
			if strings.HasPrefix(asset.MediaType, "image/") {
				image, err = assets.InspectImage(bytes.NewReader(data))
				if err != nil {
					return nil, err
				}
			}
			attachment, err := c.mediaModel.storeAttachmentBinary(ctx, m.nativeID("asset", asset.ID), roomID, asset.Filename, asset.MediaType, bytes.NewReader(data), int64(len(data)), int32(image.Width), int32(image.Height))
			if err != nil {
				return nil, fmt.Errorf("import asset storage failed")
			}
			created := newEvent(SystemActorID, &evtv1.Event{ImportContext: m.context(true, true), Event: &evtv1.Event_AssetCreated{AssetCreated: &evtv1.AssetCreatedEvent{Asset: assetFromAttachment(attachment), OriginalBinaryAvailable: true, RoomId: roomID, UserId: owner, Sha256: asset.Storage.SHA256}}})
			attached := newEvent(SystemActorID, &evtv1.Event{ImportContext: m.context(true, true), Event: &evtv1.Event_AssetAttached{AssetAttached: &evtv1.AssetAttachedEvent{AssetId: attachment.Id, RoomId: roomID, MessageEventId: messageID, UserId: owner}}})
			entries := []evtstream.BatchEntry{{Subject: evtstream.AssetAggregate(attachment.Id).SubjectFor(created), Event: created}, {Subject: evtstream.AssetAggregate(attachment.Id).SubjectFor(attached), Event: attached}}
			if err := m.commit(ctx, index, attachment.Id, false, entries); err != nil {
				return nil, err
			}
		}
		index++
	}
	entries, err := m.history(ctx)
	if err != nil {
		return nil, err
	}
	for _, batch := range entries {
		if err := m.commit(ctx, index, "", false, batch); err != nil {
			return nil, err
		}
		index++
	}
	if err := m.commit(ctx, index, "", true, nil); err != nil {
		return nil, err
	}
	if err := m.waitForViews(ctx); err != nil {
		return nil, err
	}
	result.Complete = true
	return result, nil
}

func (m *archiveImport) waitForViews(ctx context.Context) error {
	c := m.core
	if err := c.userModel.waitForUsersCurrent(ctx, "import completion", evtstream.UserSubjectFilter()); err != nil {
		return err
	}
	if err := c.userModel.waitForUserAuthCurrent(ctx, "import completion"); err != nil {
		return err
	}
	if err := c.roomModel.waitForDirectoryCurrent(ctx, c.EventPublisher); err != nil {
		return err
	}
	position, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomSubjectFilter())
	if err != nil {
		return err
	}
	if err := c.roomModel.waitForTimeline(ctx, position); err != nil {
		return err
	}
	if err := c.roomModel.waitForThreads(ctx, position); err != nil {
		return err
	}
	if err := c.roomModel.waitForReactions(ctx, position); err != nil {
		return err
	}
	if err := c.assetModel.waitForAssetsCurrent(ctx); err != nil {
		return err
	}
	return nil
}

func (m *archiveImport) preflight(ctx context.Context) error {
	c := m.core
	if err := c.roomModel.waitForDirectoryCurrent(ctx, c.EventPublisher); err != nil {
		return err
	}
	if err := c.userModel.waitForUsersCurrent(ctx, "import validation", evtstream.UserSubjectFilter()); err != nil {
		return err
	}
	users, err := c.ListUsers(ctx)
	if err != nil {
		return err
	}
	rooms, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		return err
	}
	if len(m.receipts) == 0 {
		// Existing accounts may be matched by email. Conversation history is
		// still excluded; this does not enable incremental history imports.
		posts, _, err := c.EventPublisher.SubjectEvents(ctx, "evt.room.*.message_posted")
		if err != nil {
			return err
		}
		if len(posts) > 0 {
			return invalidArgument("initial import requires a destination without messages")
		}
	}
	userNames := map[string]bool{}
	roomNames := map[string]bool{}
	for _, u := range users {
		userNames[strings.ToLower(u.Login)] = true
	}
	for _, r := range rooms {
		roomNames[canonicalRoomName(r.Name)] = true
	}
	if err := m.matchUsers(ctx); err != nil {
		return err
	}
	newAccounts := 0
	for i, u := range m.archive.Users {
		if u.Kind != "human" || u.Status != "active" {
			return invalidArgument(fmt.Sprintf("users.jsonl record %d: first version requires active humans", i+1))
		}
		match := m.userMatches[i]
		if match.account != "" || match.leader != i {
			continue
		}
		if u.Username == nil {
			return invalidArgument(fmt.Sprintf("users.jsonl record %d: new account requires a username", i+1))
		}
		newAccounts++
		name := m.username(*u.Username)
		if ValidateLogin(name) != nil || strings.TrimSpace(name) != name || ValidateDisplayName(u.DisplayName) != nil || utf8.RuneCountInString(u.DisplayName) > MaxDisplayNameLength || NormalizeDisplayName(u.DisplayName) != u.DisplayName || c.configModel.IsUsernameBlocked(name) || c.loginConflictsWithMentionHandle(name) {
			return invalidArgument(fmt.Sprintf("users.jsonl record %d: unsupported destination name", i+1))
		}
		key := strings.ToLower(name)
		if 1+i >= len(m.receipts) && userNames[key] {
			return invalidArgument(fmt.Sprintf("users.jsonl record %d: destination username collision", i+1))
		}
		userNames[key] = true
	}
	if max := c.config.Limits.MaxUsersOrDefault(); max >= 0 && newAccounts > 0 {
		count, err := c.CountUserLimitAccounts(ctx)
		if err != nil {
			return err
		}
		for i, match := range m.userMatches {
			if match.account != "" || match.leader != i {
				continue
			}
			if count >= max {
				return ErrLimitExceeded
			}
			if len(match.emails) > 0 {
				count++
			}
		}
	}
	for i, r := range m.archive.Rooms {
		name := m.channel(r.Name)
		if r.Visibility != "public" || ValidateRoomName(name) != nil || normalizeRoomName(name) != name || ValidateRoomDescription(r.Topic) != nil || ValidateRoomDescription(r.Purpose) != nil {
			return invalidArgument(fmt.Sprintf("rooms.jsonl record %d: unsupported destination channel", i+1))
		}
		key := canonicalRoomName(name)
		if 1+len(m.archive.Users)+i >= len(m.receipts) && roomNames[key] {
			return invalidArgument(fmt.Sprintf("rooms.jsonl record %d: destination channel collision", i+1))
		}
		roomNames[key] = true
	}
	for i, e := range m.archive.Events {
		d := e.Data
		if len(d.Content.Body.Text) > MaxMessageBodyLength || len(d.Content.AssetIDs) > MaxMessageAttachmentAssetIDs || (e.Type == "message.snapshot" && d.CreatedAt == nil) || d.Content.Status == "unavailable" {
			return invalidArgument(fmt.Sprintf("events.jsonl record %d: unsupported content or unknown creation time", i+1))
		}
		if strings.HasPrefix(e.Type, "reaction.") {
			if _, err := importEmoji(d.Emoji); err != nil {
				return invalidArgument(fmt.Sprintf("events.jsonl record %d: unsupported emoji", i+1))
			}
		}
		for _, r := range d.Reactions {
			if _, err := importEmoji(r.Emoji); err != nil {
				return invalidArgument(fmt.Sprintf("events.jsonl record %d: unsupported emoji", i+1))
			}
		}
		if e.Type == "room.name_changed" && ValidateRoomName(m.channel(d.Value)) != nil {
			return invalidArgument("historical channel name is unsupported")
		}
		if (e.Type == "room.topic_changed" || e.Type == "room.purpose_changed") && ValidateRoomDescription(d.Value) != nil {
			return invalidArgument("historical channel metadata is too long")
		}
	}
	messageAuthors := map[string]string{}
	for _, e := range m.archive.Events {
		if e.Data.AuthorID != "" {
			messageAuthors[e.Data.MessageID] = e.Data.AuthorID
		}
	}
	m.assetBindings = map[string]importAssetBinding{}
	for _, e := range m.archive.Events {
		for _, id := range e.Data.Content.AssetIDs {
			if previous, ok := m.assetBindings[id]; ok && previous.message != e.Data.MessageID {
				return invalidArgument("first version requires each file to belong to one message")
			}
			m.assetBindings[id] = importAssetBinding{messageAuthors[e.Data.MessageID], e.RoomID, e.Data.MessageID}
		}
	}
	for i, a := range m.archive.Assets {
		if a.Storage.Status != "available" {
			return invalidArgument(fmt.Sprintf("assets.jsonl record %d: unavailable files are not yet supported", i+1))
		}
		if _, ok := m.assetBindings[a.ID]; !ok {
			return invalidArgument(fmt.Sprintf("assets.jsonl record %d: file must belong to exactly one message", i+1))
		}
		if strings.HasPrefix(a.MediaType, "image/") {
			if _, err := assets.InspectImage(bytes.NewReader(m.archive.Files[a.Storage.Path])); err != nil {
				return invalidArgument(fmt.Sprintf("assets.jsonl record %d: unsupported image", i+1))
			}
		}
	}
	return nil
}

func (m *archiveImport) assetOwner(id string) (string, string, string) {
	binding := m.assetBindings[id]
	return m.users[binding.author], m.rooms[binding.room], m.nativeID("message", binding.message)
}

func importTime(value *string) *timestamppb.Timestamp {
	if value == nil {
		return timestamppb.Now()
	}
	t, _ := time.Parse(time.RFC3339Nano, *value)
	return timestamppb.New(t)
}
