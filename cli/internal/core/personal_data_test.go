package core

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
	personaldatav1 "hmans.de/chatto/internal/pb/chatto/personaldata/v1"
)

func TestPersonalDataPreferencesRoundTrip(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := "personal-data-user"

	empty, err := chatto.PersonalData.GetPreferences(ctx, userID)
	if err != nil {
		t.Fatalf("GetPreferences empty: %v", err)
	}
	if empty.Locale != nil || empty.Timezone != nil || empty.TimeFormat != nil {
		t.Fatalf("empty preferences = %v", empty)
	}

	locale := "en-GB"
	timezone := "Europe/Berlin"
	format := personaldatav1.TimeFormat_TIME_FORMAT_24_HOUR
	updated, err := chatto.PersonalData.UpdatePreferences(ctx, userID, func(preferences *personaldatav1.Preferences) error {
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

	stored, err := chatto.PersonalData.GetPreferences(ctx, userID)
	if err != nil {
		t.Fatalf("GetPreferences stored: %v", err)
	}
	if stored.GetLocale() != locale || stored.GetTimezone() != timezone || stored.GetTimeFormat() != format {
		t.Fatalf("stored preferences = %v", stored)
	}
}

func TestPersonalDataServerDirectoryProtectsHomeServer(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := "personal-directory-user"
	serverA := &personaldatav1.KnownServer{Id: "a", Url: "https://a.example", Name: "A", AddedAt: timestamppb.Now()}
	serverB := &personaldatav1.KnownServer{Id: "b", Url: "https://b.example", Name: "B", AddedAt: timestamppb.Now()}

	if _, err := chatto.PersonalData.CreateServer(ctx, userID, serverA); err != nil {
		t.Fatalf("CreateServer a: %v", err)
	}
	directory, err := chatto.PersonalData.ListServers(ctx, userID)
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if directory.GetHomeServerId() != "a" {
		t.Fatalf("first server home = %q, want a", directory.GetHomeServerId())
	}
	if err := chatto.PersonalData.DeleteServer(ctx, userID, "a"); !errors.Is(err, ErrCannotDeleteHomeServer) {
		t.Fatalf("DeleteServer home err = %v, want ErrCannotDeleteHomeServer", err)
	}

	if _, err := chatto.PersonalData.CreateServer(ctx, userID, serverB); err != nil {
		t.Fatalf("CreateServer b: %v", err)
	}
	if _, err := chatto.PersonalData.SetHomeServer(ctx, userID, "b"); err != nil {
		t.Fatalf("SetHomeServer b: %v", err)
	}
	if err := chatto.PersonalData.DeleteServer(ctx, userID, "a"); err != nil {
		t.Fatalf("DeleteServer a after move: %v", err)
	}
	directory, err = chatto.PersonalData.ListServers(ctx, userID)
	if err != nil {
		t.Fatalf("ListServers final: %v", err)
	}
	if directory.GetHomeServerId() != "b" || len(directory.GetServers()) != 1 || directory.GetServers()[0].GetId() != "b" {
		t.Fatalf("final directory = %v", directory)
	}
}

func TestPersonalDataRejectsDuplicateServerID(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	server := &personaldatav1.KnownServer{Id: "same", Url: "https://one.example", Name: "One"}
	if _, err := chatto.PersonalData.CreateServer(ctx, "user", server); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if _, err := chatto.PersonalData.CreateServer(ctx, "user", server); !errors.Is(err, ErrPersonalServerAlreadyExists) {
		t.Fatalf("duplicate err = %v, want ErrPersonalServerAlreadyExists", err)
	}
}

func TestPersonalDataRejectsDuplicateServerURL(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	first := &personaldatav1.KnownServer{Id: "first", Url: "https://same.example", Name: "First"}
	duplicate := &personaldatav1.KnownServer{Id: "second", Url: first.Url, Name: "Second"}
	if _, err := chatto.PersonalData.CreateServer(ctx, "user", first); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if _, err := chatto.PersonalData.CreateServer(ctx, "user", duplicate); !errors.Is(err, ErrPersonalServerAlreadyExists) {
		t.Fatalf("duplicate URL err = %v, want ErrPersonalServerAlreadyExists", err)
	}

	second := &personaldatav1.KnownServer{Id: "second", Url: "https://other.example", Name: "Second"}
	if _, err := chatto.PersonalData.CreateServer(ctx, "user", second); err != nil {
		t.Fatalf("CreateServer second: %v", err)
	}
	if _, err := chatto.PersonalData.UpdateServer(ctx, "user", second.Id, func(server *personaldatav1.KnownServer) error {
		server.Url = first.Url
		return nil
	}); !errors.Is(err, ErrPersonalServerAlreadyExists) {
		t.Fatalf("duplicate URL update err = %v, want ErrPersonalServerAlreadyExists", err)
	}
}
