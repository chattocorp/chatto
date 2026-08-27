//go:build test_endpoints

package http_server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"hmans.de/chatto/internal/core"
)

func TestPerformanceFixtureEndpointSeedsEncryptedLargeState(t *testing.T) {
	server, client, chattoCore, _ := setupTestHTTPServerWithMailer(t)
	ctx := testContext(t)
	if err := chattoCore.SeedDefaultRooms(ctx); err != nil {
		t.Fatalf("SeedDefaultRooms: %v", err)
	}

	body, err := json.Marshal(map[string]int{
		"users":     4,
		"messages":  20,
		"batchSize": 5,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	response, err := client.Post(server.URL+"/auth/test/seed-performance", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST seed-performance: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}

	var result struct {
		Version         string `json:"version"`
		SyntheticUsers  int    `json:"syntheticUsers"`
		Messages        int    `json:"messages"`
		RoomID          string `json:"roomId"`
		LastUserLogin   string `json:"lastUserLogin"`
		LastMessageID   string `json:"lastMessageId"`
		LastMessageBody string `json:"lastMessageBody"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Version != core.PerformanceFixtureVersion || result.SyntheticUsers != 4 || result.Messages != 20 {
		t.Fatalf("fixture result = %+v", result)
	}
	if result.RoomID == "" || result.LastUserLogin != "perfuser0004" || result.LastMessageID == "" {
		t.Fatalf("fixture identifiers = %+v", result)
	}
	if count, err := chattoCore.CountUsers(ctx); err != nil || count != 4 {
		t.Fatalf("CountUsers = %d, %v; want 4", count, err)
	}
	plaintext, err := chattoCore.GetMessageBody(ctx, result.LastMessageID)
	if err != nil {
		t.Fatalf("GetMessageBody: %v", err)
	}
	if plaintext != result.LastMessageBody {
		t.Fatalf("last body = %q, want %q", plaintext, result.LastMessageBody)
	}

	duplicate, err := client.Post(server.URL+"/auth/test/seed-performance", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("duplicate POST seed-performance: %v", err)
	}
	defer duplicate.Body.Close()
	if duplicate.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", duplicate.StatusCode)
	}
}
