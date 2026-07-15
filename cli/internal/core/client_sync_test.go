package core

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
	clientsyncv1 "hmans.de/chatto/internal/pb/chatto/clientsync/v1"
)

func TestClientSyncPreferencesRoundTrip(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := "client-sync-user"

	empty, err := chatto.ClientSync.GetPreferences(ctx, userID)
	if err != nil {
		t.Fatalf("GetPreferences empty: %v", err)
	}
	if empty.Locale != nil || empty.Timezone != nil || empty.TimeFormat != nil {
		t.Fatalf("empty preferences = %v", empty)
	}

	locale := "en-GB"
	timezone := "Europe/Berlin"
	format := clientsyncv1.TimeFormat_TIME_FORMAT_24_HOUR
	updated, err := chatto.ClientSync.UpdatePreferences(ctx, userID, func(preferences *clientsyncv1.Preferences) error {
		preferences.Locale = &locale
		preferences.Timezone = &timezone
		preferences.TimeFormat = &format
		return nil
	})
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if updated.GetLocale() != locale || updated.GetTimezone() != timezone || updated.GetTimeFormat() != format {
		t.Fatalf("updated preferences = %v", updated)
	}

	stored, err := chatto.ClientSync.GetPreferences(ctx, userID)
	if err != nil {
		t.Fatalf("GetPreferences stored: %v", err)
	}
	if stored.GetLocale() != locale || stored.GetTimezone() != timezone || stored.GetTimeFormat() != format {
		t.Fatalf("stored preferences = %v", stored)
	}
}

func TestClientSyncServerDirectoryProtectsHomeServer(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := "personal-directory-user"
	serverA := &clientsyncv1.KnownServer{Id: "a", Url: "https://a.example", Name: "A", AddedAt: timestamppb.Now()}
	serverB := &clientsyncv1.KnownServer{Id: "b", Url: "https://b.example", Name: "B", AddedAt: timestamppb.Now()}

	if _, err := chatto.ClientSync.CreateServer(ctx, userID, serverA); err != nil {
		t.Fatalf("CreateServer a: %v", err)
	}
	directory, err := chatto.ClientSync.ListServers(ctx, userID)
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if directory.GetHomeServerId() != "a" {
		t.Fatalf("first server home = %q, want a", directory.GetHomeServerId())
	}
	if err := chatto.ClientSync.DeleteServer(ctx, userID, "a"); !errors.Is(err, ErrCannotDeleteHomeServer) {
		t.Fatalf("DeleteServer home err = %v, want ErrCannotDeleteHomeServer", err)
	}

	if _, err := chatto.ClientSync.CreateServer(ctx, userID, serverB); err != nil {
		t.Fatalf("CreateServer b: %v", err)
	}
	if _, err := chatto.ClientSync.SetHomeServer(ctx, userID, "b"); err != nil {
		t.Fatalf("SetHomeServer b: %v", err)
	}
	if err := chatto.ClientSync.DeleteServer(ctx, userID, "a"); err != nil {
		t.Fatalf("DeleteServer a after move: %v", err)
	}
	directory, err = chatto.ClientSync.ListServers(ctx, userID)
	if err != nil {
		t.Fatalf("ListServers final: %v", err)
	}
	if directory.GetHomeServerId() != "b" || len(directory.GetServers()) != 1 || directory.GetServers()[0].GetId() != "b" {
		t.Fatalf("final directory = %v", directory)
	}
}

func TestClientSyncRejectsDuplicateServerID(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	server := &clientsyncv1.KnownServer{Id: "same", Url: "https://one.example", Name: "One"}
	if _, err := chatto.ClientSync.CreateServer(ctx, "user", server); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if _, err := chatto.ClientSync.CreateServer(ctx, "user", server); !errors.Is(err, ErrKnownServerAlreadyExists) {
		t.Fatalf("duplicate err = %v, want ErrKnownServerAlreadyExists", err)
	}
}

func TestClientSyncRejectsDuplicateServerURL(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	first := &clientsyncv1.KnownServer{Id: "first", Url: "https://same.example", Name: "First"}
	duplicate := &clientsyncv1.KnownServer{Id: "second", Url: first.Url, Name: "Second"}
	if _, err := chatto.ClientSync.CreateServer(ctx, "user", first); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if _, err := chatto.ClientSync.CreateServer(ctx, "user", duplicate); !errors.Is(err, ErrKnownServerAlreadyExists) {
		t.Fatalf("duplicate URL err = %v, want ErrKnownServerAlreadyExists", err)
	}

	second := &clientsyncv1.KnownServer{Id: "second", Url: "https://other.example", Name: "Second"}
	if _, err := chatto.ClientSync.CreateServer(ctx, "user", second); err != nil {
		t.Fatalf("CreateServer second: %v", err)
	}
	if _, err := chatto.ClientSync.UpdateServer(ctx, "user", second.Id, func(server *clientsyncv1.KnownServer) error {
		server.Url = first.Url
		return nil
	}); !errors.Is(err, ErrKnownServerAlreadyExists) {
		t.Fatalf("duplicate URL update err = %v, want ErrKnownServerAlreadyExists", err)
	}
}
