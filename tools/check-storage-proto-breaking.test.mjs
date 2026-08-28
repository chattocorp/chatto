import assert from "node:assert/strict";
import test from "node:test";

import { isAllowedStorageBreakingDiagnostic } from "./check-storage-proto-breaking.mjs";

const allowedMessages = [
  "ServerUserPreferencesUpdatedEvent",
  "UserCreatedEvent",
  "UserDeletedEvent",
  "UserProfileUpdatedEvent",
];

for (const message of allowedMessages) {
  test(`allows removal of transient ${message}`, () => {
    assert.equal(
      isAllowedStorageBreakingDiagnostic({
        path: "chatto/core/v1/user_events.proto",
        type: "MESSAGE_NO_DELETE",
        message: `Previously present message "${message}" was deleted from file.`,
      }),
      true,
    );
  });
}

test("rejects removal of a durable user event", () => {
  assert.equal(
    isAllowedStorageBreakingDiagnostic({
      path: "chatto/core/v1/user_events.proto",
      type: "MESSAGE_NO_DELETE",
      message:
        'Previously present message "UserAccountCreatedEvent" was deleted from file.',
    }),
    false,
  );
});

test("rejects other breaking rules for an allowed message", () => {
  assert.equal(
    isAllowedStorageBreakingDiagnostic({
      path: "chatto/core/v1/user_events.proto",
      type: "FIELD_NO_DELETE",
      message:
        'Previously present message "UserCreatedEvent" was deleted from file.',
    }),
    false,
  );
});
