package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	op "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1/operatorv1connect"
)

type cliArchiveImport struct {
	archive                         *interchange.Archive
	naming                          importNaming
	progress                        *importProgress
	http                            *http.Client
	userAPI                         operatorv1connect.OperatorUserServiceClient
	roomAPI                         operatorv1connect.OperatorRoomServiceClient
	messageAPI                      operatorv1connect.OperatorMessageServiceClient
	assetAPI                        operatorv1connect.OperatorAssetServiceClient
	users, rooms, messages, authors map[string]string
	assets                          map[string]interchange.Asset
	userMatches                     []importUserMatch
}

// importRef keeps arbitrary source identifiers private and unambiguous in keys.
func importRef(id string) string { sum := sha256.Sum256([]byte(id)); return hex.EncodeToString(sum[:]) }

func newCLIArchiveImport(a *interchange.Archive, naming importNaming, path string) (*cliArchiveImport, error) {
	config, err := resolveOperatorAPIClientConfig()
	if err != nil {
		return nil, err
	}
	options, _ := json.Marshal(naming)
	sum := sha256.Sum256([]byte(a.Digest() + "\x00" + config.socketPath + "\x00" + string(options)))
	p, err := openImportProgress(path, hex.EncodeToString(sum[:]))
	if err != nil {
		return nil, err
	}
	h, base, err := newOperatorHTTPClient()
	if err != nil {
		p.file.Close()
		return nil, err
	}
	m := &cliArchiveImport{archive: a, naming: naming, progress: p, http: h, users: map[string]string{}, rooms: map[string]string{}, messages: map[string]string{}, authors: map[string]string{}, assets: map[string]interchange.Asset{}}
	m.userAPI = operatorv1connect.NewOperatorUserServiceClient(h, base)
	m.roomAPI = operatorv1connect.NewOperatorRoomServiceClient(h, base)
	m.messageAPI = operatorv1connect.NewOperatorMessageServiceClient(h, base)
	m.assetAPI = operatorv1connect.NewOperatorAssetServiceClient(h, base)
	for _, a := range a.Assets {
		m.assets[a.ID] = a
	}
	return m, nil
}

func (m *cliArchiveImport) close() { m.http.CloseIdleConnections(); _ = m.progress.file.Close() }
func (m *cliArchiveImport) channel(s string) string {
	return m.naming.ChannelPrefix + s + m.naming.ChannelSuffix
}
func (m *cliArchiveImport) username(s string) string {
	return m.naming.UsernamePrefix + s + m.naming.UsernameSuffix
}
func sortedImportKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// run resolves identities before writes, then applies each source operation in
// order. A durable local mapping is written before dependent calls are made.
func (m *cliArchiveImport) run(ctx context.Context, validateOnly, acceptOmissions bool) error {
	if m.progress.values["complete"] == "true" {
		return nil
	}
	if err := m.preflight(ctx); err != nil {
		return err
	}
	if validateOnly {
		return nil
	}
	if len(m.archive.Manifest.Omissions) > 0 && !acceptOmissions {
		return errors.New("archive declares omissions; pass --accept-omissions")
	}
	for i, u := range m.archive.Users {
		key := "user/" + importRef(u.ID)
		match := m.userMatches[i]
		id := m.progress.values[key]
		if id == "" {
			id = match.account
			if match.leader < i {
				id = m.users[m.archive.Users[match.leader].ID]
			}
			if id == "" {
				if err := m.progress.put("pending/"+key, "true"); err != nil {
					return err
				}
				email := ""
				if len(match.emails) > 0 {
					email = match.emails[0]
				}
				res, err := m.userAPI.CreateUser(ctx, operatorRequest(&op.CreateUserRequest{Login: m.username(*u.Username), DisplayName: u.DisplayName, VerifiedEmail: email, OperationKey: m.progress.operation(key)}))
				if err != nil {
					return fmt.Errorf("import user record %d: %w", i+1, err)
				}
				id = res.Msg.GetMember().GetUser().GetId()
			}
			if err := m.progress.put(key, id); err != nil {
				return err
			}
		}
		m.users[u.ID] = id
		if match.leader == i {
			for _, email := range match.emails {
				if _, err := m.userAPI.AddVerifiedEmail(ctx, operatorRequest(&op.AddVerifiedEmailRequest{UserId: id, Email: email})); err != nil {
					return err
				}
			}
		}
	}
	for i, r := range m.archive.Rooms {
		key := "room/" + importRef(r.ID)
		id := m.progress.values[key]
		if id == "" {
			if err := m.progress.put("pending/"+key, "true"); err != nil {
				return err
			}
			res, err := m.roomAPI.CreateRoom(ctx, operatorRequest(&op.CreateRoomRequest{Name: m.channel(r.Name), Description: r.Topic, OperationKey: m.progress.operation(key)}))
			if err != nil {
				return fmt.Errorf("import channel record %d: %w", i+1, err)
			}
			id = res.Msg.GetRoom().GetId()
			if err = m.progress.put(key, id); err != nil {
				return err
			}
		}
		m.rooms[r.ID] = id
	}
	if err := m.history(ctx); err != nil {
		return err
	}
	return m.progress.put("complete", "true")
}

func (m *cliArchiveImport) preflight(ctx context.Context) error {
	if err := m.matchUsers(ctx); err != nil {
		return err
	}
	userNames := map[string]bool{}
	roomNames := map[string]bool{}
	for i, u := range m.archive.Users {
		if u.Kind != "human" || u.Status != "active" {
			return fmt.Errorf("users.jsonl record %d: only active human accounts are supported", i+1)
		}
		match := m.userMatches[i]
		if match.account != "" || match.leader < i || m.progress.values["user/"+importRef(u.ID)] != "" {
			continue
		}
		if u.Username == nil {
			return fmt.Errorf("users.jsonl record %d: new account needs a username", i+1)
		}
		name := m.username(*u.Username)
		if core.ValidateLogin(name) != nil || strings.TrimSpace(name) != name || core.ValidateDisplayName(u.DisplayName) != nil {
			return fmt.Errorf("users.jsonl record %d: unsupported destination name", i+1)
		}
		lower := strings.ToLower(name)
		if userNames[lower] {
			return errors.New("import: duplicate destination username")
		}
		userNames[lower] = true
		// A pending keyed creation can already exist after a lost response.
		// Reissuing it below distinguishes our creation from a name collision.
		_, err := m.userAPI.GetUser(ctx, operatorRequest(&op.GetUserRequest{Login: name}))
		if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
			return err
		}
		if err == nil && m.progress.values["pending/user/"+importRef(u.ID)] == "" {
			return fmt.Errorf("users.jsonl record %d: destination username collision", i+1)
		}
	}
	for i, r := range m.archive.Rooms {
		name := m.channel(r.Name)
		if r.Visibility != "public" || core.ValidateRoomName(name) != nil || strings.TrimSpace(name) != name || core.ValidateRoomDescription(r.Topic) != nil || core.ValidateRoomDescription(r.Purpose) != nil {
			return fmt.Errorf("rooms.jsonl record %d: unsupported channel", i+1)
		}
		lower := strings.ToLower(name)
		if roomNames[lower] {
			return errors.New("import: duplicate destination channel")
		}
		roomNames[lower] = true
		res, err := m.roomAPI.ListRooms(ctx, operatorRequest(&op.ListRoomsRequest{Name: name}))
		if err != nil {
			return err
		}
		if len(res.Msg.Rooms) > 0 && m.progress.values["room/"+importRef(r.ID)] == "" && m.progress.values["pending/room/"+importRef(r.ID)] == "" {
			return fmt.Errorf("rooms.jsonl record %d: destination channel collision", i+1)
		}
	}
	for i, a := range m.archive.Assets {
		if a.Storage.Status != "available" {
			return fmt.Errorf("assets.jsonl record %d: unavailable files are not supported", i+1)
		}
	}
	usedAssets := map[string]bool{}
	err := m.archive.VisitEvents(func(e interchange.Event) error {
		d := e.Data
		for _, id := range d.Content.AssetIDs {
			usedAssets[id] = true
		}
		if strings.HasPrefix(e.Type, "reaction.") {
			if _, err := core.HistoricalReactionName(d.Emoji); err != nil {
				return err
			}
		}
		for _, r := range d.Reactions {
			if _, err := core.HistoricalReactionName(r.Emoji); err != nil {
				return err
			}
		}
		if len(d.Content.Body.Text) > core.MaxMessageBodyLength || len(d.Content.AssetIDs) > core.MaxMessageAttachmentAssetIDs || d.Content.Status == "unavailable" || (e.Type == "message.snapshot" && d.CreatedAt == nil) {
			return errors.New("unsupported content or unknown message creation time")
		}
		if d.AuthorID != "" {
			m.authors[d.MessageID] = d.AuthorID
		}
		if e.Type == "room.name_changed" && core.ValidateRoomName(m.channel(d.Value)) != nil {
			return errors.New("unsupported historical channel name")
		}
		if (e.Type == "room.topic_changed" || e.Type == "room.purpose_changed") && core.ValidateRoomDescription(d.Value) != nil {
			return errors.New("unsupported historical channel metadata")
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, a := range m.archive.Assets {
		if !usedAssets[a.ID] {
			return errors.New("import: asset has no message reference")
		}
	}
	return nil
}
