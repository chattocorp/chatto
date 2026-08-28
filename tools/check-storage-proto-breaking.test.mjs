import assert from "node:assert/strict";
import test from "node:test";

import { isExactCorePackageRelocation } from "./check-storage-proto-breaking.mjs";

const relocatedFiles = [
  "asset_events.proto",
  "auth_events.proto",
  "authorization_events.proto",
  "config_events.proto",
  "credential_usage.proto",
  "encryption_keys.proto",
  "event.proto",
  "invitation_events.proto",
  "message_events.proto",
  "models.proto",
  "moderation_events.proto",
  "notification.proto",
  "oauth_client_events.proto",
  "push.proto",
  "rbac_events.proto",
  "reaction_events.proto",
  "room_events.proto",
  "room_group_events.proto",
  "thread_events.proto",
  "user_events.proto",
  "user_preferences.proto",
];

function relocationDiagnostic(file) {
  return {
    type: "FILE_NO_DELETE",
    message: `Previously present file "chatto/core/v1/${file}" was deleted.`,
  };
}

test("allows the exact core package relocation", () => {
  assert.equal(
    isExactCorePackageRelocation(relocatedFiles.map(relocationDiagnostic)),
    true,
  );
});

test("rejects an incomplete core package relocation", () => {
  assert.equal(
    isExactCorePackageRelocation(
      relocatedFiles.slice(1).map(relocationDiagnostic),
    ),
    false,
  );
});

test("rejects an unrelated deleted file", () => {
  const diagnostics = relocatedFiles.map(relocationDiagnostic);
  diagnostics[0] = relocationDiagnostic("surprise.proto");
  assert.equal(isExactCorePackageRelocation(diagnostics), false);
});

test("rejects another breaking rule", () => {
  const diagnostics = relocatedFiles.map(relocationDiagnostic);
  diagnostics[0] = {
    type: "FIELD_NO_DELETE",
    message: 'Previously present field "string id" on message "Event" was deleted.',
  };
  assert.equal(isExactCorePackageRelocation(diagnostics), false);
});
