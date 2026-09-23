package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"hmans.de/chatto/internal/core"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

func TestOperatorMessageImportBodySourcesAndAttachment(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "cli-import-author", "Import Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "cli-import-room", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "body.txt")
	if err := os.WriteFile(path, []byte("line one\nline two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"operator", "message", "import", "--room-id", room.Id, "--author-id", author.Id, "--created-at", "2022-08-18T11:11:11.147Z"}
	output := env.run(t, append(append([]string(nil), args...), "--body-file", path, "--preview-url", "https://example.invalid", "--preview-title", "Saved title", "--json")...)
	var result operatorv1.ImportMessageResponse
	if err := protojson.Unmarshal([]byte(output), &result); err != nil || result.GetMessageId() == "" {
		t.Fatalf("import JSON = %q, err = %v", output, err)
	}
	body, err := env.core.GetFullMessageBody(env.ctx, result.GetMessageId())
	if err != nil || body == nil || body.Body != "line one\nline two\n" || body.LinkPreview.GetTitle() != "Saved title" {
		t.Fatalf("file body = %+v, err = %v", body, err)
	}
	stdinID := strings.TrimSpace(env.run(t, append(append([]string(nil), args...), "--body-stdin", "--in-reply-to", result.GetMessageId())...))
	stdinBody, err := env.core.GetFullMessageBody(env.ctx, stdinID)
	if err != nil || stdinBody == nil || stdinBody.Body != "password123\n" {
		t.Fatalf("stdin body = %+v, err = %v", stdinBody, err)
	}
	assetPath := filepath.Join(t.TempDir(), "attachment.txt")
	if err := os.WriteFile(assetPath, []byte("attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	assetID := strings.TrimSpace(env.run(t, "operator", "asset", "upload", "--room-id", room.Id, "--author-id", author.Id, "--file", assetPath))
	attachmentID := strings.TrimSpace(env.run(t, append(append([]string(nil), args...), "--attachment-asset-id", assetID)...))
	attachmentBody, err := env.core.GetFullMessageBody(env.ctx, attachmentID)
	if err != nil || attachmentBody == nil || attachmentBody.Body != "" || len(attachmentBody.Attachments) != 1 || attachmentBody.Attachments[0].GetId() != assetID {
		t.Fatalf("attachment-only body = %+v, err = %v", attachmentBody, err)
	}
}

func TestOperatorMessageImportRejectsConflictingBodyFlags(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	args := []string{"operator", "message", "import", "--room-id", "room", "--author-id", "user", "--created-at", "2022-08-18T11:11:11Z"}
	for _, flags := range [][]string{
		{"--body", "text", "--body-stdin"},
		{"--body", "text", "--body-file", "unused.txt"},
		{"--body-file", "unused.txt", "--body-stdin"},
		{"--body-file", ""},
	} {
		if _, err := env.execute(t, append(append([]string(nil), args...), flags...)...); err == nil {
			t.Fatalf("accepted conflicting flags %v", flags)
		}
	}
}
