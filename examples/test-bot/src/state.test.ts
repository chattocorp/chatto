import assert from "node:assert/strict";
import { mkdtemp, rm, stat } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  loadTestBotState,
  rememberProcessedEvent,
  saveTestBotState,
  type TestBotState,
} from "./state.js";

test("state round-trips with owner-only file permissions", async (t) => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "chatto-test-bot-"));
  t.after(() => rm(directory, { force: true, recursive: true }));
  const stateFile = path.join(directory, "nested", "state.json");
  await saveTestBotState(stateFile, {
    resumeCursor: "opaque-cursor",
    processedEventIds: ["event-1", "event-2"],
    followedThreadKeys: ["room-1\u0000root-1"],
  });

  assert.deepEqual(await loadTestBotState(stateFile), {
    resumeCursor: "opaque-cursor",
    processedEventIds: ["event-1", "event-2"],
    followedThreadKeys: ["room-1\u0000root-1"],
  });
  assert.equal((await stat(stateFile)).mode & 0o777, 0o600);
});

test("processed event IDs are deduplicated and bounded", () => {
  const state: TestBotState = {
    processedEventIds: [],
    followedThreadKeys: [],
  };
  assert.equal(rememberProcessedEvent(state, "event-1"), true);
  assert.equal(rememberProcessedEvent(state, "event-1"), false);
  for (let index = 2; index <= 2_100; index += 1) {
    rememberProcessedEvent(state, `event-${index}`);
  }
  assert.equal(state.processedEventIds.length, 2_048);
  assert.equal(state.processedEventIds.at(-1), "event-2100");
  assert.equal(state.processedEventIds.includes("event-1"), false);
});
