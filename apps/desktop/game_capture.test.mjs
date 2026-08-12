// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import assert from "node:assert/strict";
import test from "node:test";
import {
  parseGameCapturePublisherRequest,
  parseGameCapturePublisherStatus,
  parseMacOSGameCaptureSourceId,
  parseMacOSGameCaptureSources,
} from "./game_capture.mjs";

test("maps macOS windows to opaque renderer capture sources", () => {
  assert.deepEqual(
    parseMacOSGameCaptureSources(
      JSON.stringify({
        protocolVersion: 1,
        windows: [
          {
            windowID: 42,
            applicationName: "Example Game",
            title: "Example Game — Main Menu",
            bundleIdentifier: "com.example.game",
            width: 1920,
            height: 1080,
          },
        ],
      }),
    ),
    {
      protocolVersion: 1,
      sources: [
        {
          id: "window:42",
          kind: "window",
          applicationName: "Example Game",
          title: "Example Game — Main Menu",
          width: 1920,
          height: 1080,
        },
      ],
    },
  );
});

test("rejects unsupported or malformed helper responses", () => {
  assert.throws(
    () =>
      parseMacOSGameCaptureSources(
        JSON.stringify({ protocolVersion: 2, windows: [] }),
      ),
    /unsupported source list/,
  );
  assert.throws(
    () =>
      parseMacOSGameCaptureSources(
        JSON.stringify({
          protocolVersion: 1,
          windows: [
            {
              windowID: 0,
              applicationName: "Example Game",
              title: "",
              width: 1920,
              height: 1080,
            },
          ],
        }),
      ),
    /invalid window source/,
  );
});

test("resolves only valid opaque macOS window sources", () => {
  assert.equal(parseMacOSGameCaptureSourceId("window:42"), 42);
  assert.throws(() => parseMacOSGameCaptureSourceId("display:42"), /invalid/);
  assert.throws(() => parseMacOSGameCaptureSourceId("window:0"), /invalid/);
});

test("validates native publisher credentials without changing them", () => {
  const request = {
    sourceId: "window:42",
    livekitUrl: "wss://livekit.example.test",
    token: "secret-token",
    e2eeKey: "secret-key",
  };
  assert.deepEqual(parseGameCapturePublisherRequest(request), request);
  assert.throws(
    () =>
      parseGameCapturePublisherRequest({
        ...request,
        livekitUrl: "file:///tmp/socket",
      }),
    /LiveKit URL/,
  );
  assert.throws(
    () => parseGameCapturePublisherRequest({ ...request, token: "" }),
    /request is invalid/,
  );
});

test("parses native publisher lifecycle status", () => {
  assert.deepEqual(
    parseGameCapturePublisherStatus(
      JSON.stringify({
        protocolVersion: 1,
        kind: "started",
        width: 1920,
        height: 1080,
        frameRate: 60,
      }),
    ),
    { kind: "started", width: 1920, height: 1080, frameRate: 60 },
  );
});
