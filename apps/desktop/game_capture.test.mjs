// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import assert from "node:assert/strict";
import test from "node:test";
import {
  MacOSGameCaptureSourceOffers,
  parseGameCapturePublisherRequest,
  parseGameCapturePublisherStatus,
  parseMacOSGameCaptureSourceId,
  parseMacOSGameCaptureSources,
  supportsMacOSGameCapture,
} from "./game_capture.mjs";

test("advertises native game capture only on supported macOS versions", () => {
  assert.equal(supportsMacOSGameCapture("12.7.6"), false);
  assert.equal(supportsMacOSGameCapture("14.7.6"), false);
  assert.equal(supportsMacOSGameCapture("15"), true);
  assert.equal(supportsMacOSGameCapture("15.0"), true);
  assert.equal(supportsMacOSGameCapture("16.1.2"), true);
  assert.equal(supportsMacOSGameCapture("Version 15.0"), false);
  assert.equal(supportsMacOSGameCapture("15.0-beta"), false);
  assert.equal(supportsMacOSGameCapture(""), false);
  assert.equal(supportsMacOSGameCapture(undefined), false);
});

test("parses macOS window and display capture sources", () => {
  const windowPreview = Buffer.from([0xff, 0xd8, 0xff]);
  const displayPreview = Buffer.from([0xff, 0xd8]);
  assert.deepEqual(
    parseMacOSGameCaptureSources(
      sourcePreviewFrame(
        {
          protocolVersion: 1,
          sources: [
            {
              kind: "display",
              nativeID: 7,
              displayIndex: 1,
              isMainDisplay: true,
              width: 2560,
              height: 1440,
              previewByteLength: displayPreview.length,
            },
            {
              kind: "window",
              nativeID: 42,
              applicationName: "Example Game",
              title: "Example Game — Main Menu",
              bundleIdentifier: "com.example.game",
              width: 1920,
              height: 1080,
              previewByteLength: windowPreview.length,
            },
          ],
        },
        [displayPreview, windowPreview],
      ),
    ),
    {
      protocolVersion: 1,
      sources: [
        {
          id: "display:7",
          kind: "display",
          displayIndex: 1,
          isMainDisplay: true,
          width: 2560,
          height: 1440,
          preview: Uint8Array.from(displayPreview),
        },
        {
          id: "window:42",
          kind: "window",
          applicationName: "Example Game",
          bundleIdentifier: "com.example.game",
          title: "Example Game — Main Menu",
          width: 1920,
          height: 1080,
          preview: Uint8Array.from(windowPreview),
        },
      ],
    },
  );
});

test("rejects unsupported or malformed helper responses", () => {
  assert.throws(
    () =>
      parseMacOSGameCaptureSources(
        sourcePreviewFrame({ protocolVersion: 2, sources: [] }),
      ),
    /unsupported source list/,
  );
  assert.throws(
    () =>
      parseMacOSGameCaptureSources(
        sourcePreviewFrame({
          protocolVersion: 1,
          sources: [
            {
              kind: "window",
              nativeID: 0,
              applicationName: "Example Game",
              bundleIdentifier: "com.example.game",
              title: "",
              width: 1920,
              height: 1080,
              previewByteLength: 0,
            },
          ],
        }),
      ),
    /invalid capture source/,
  );
  assert.throws(
    () =>
      parseMacOSGameCaptureSources(
        sourcePreviewFrame({
          protocolVersion: 1,
          sources: [
            {
              kind: "window",
              nativeID: 42,
              applicationName: "Example Game",
              bundleIdentifier: "com.example.game",
              title: "",
              width: 1920,
              height: 1080,
              previewByteLength: 10,
            },
          ],
        }),
      ),
    /truncated preview data/,
  );
});

test("resolves only valid helper-native macOS source IDs", () => {
  assert.deepEqual(parseMacOSGameCaptureSourceId("window:42"), {
    kind: "window",
    nativeId: 42,
  });
  assert.deepEqual(parseMacOSGameCaptureSourceId("display:7"), {
    kind: "display",
    nativeId: 7,
  });
  assert.throws(() => parseMacOSGameCaptureSourceId("window:0"), /invalid/);
  assert.throws(
    () => parseMacOSGameCaptureSourceId("application:42"),
    /invalid/,
  );
});

test("offers native sources through single-use opaque tokens", () => {
  let now = 1_000;
  let nextToken = 0;
  const offers = new MacOSGameCaptureSourceOffers({
    now: () => now,
    createToken: () => `opaque-${++nextToken}`,
  });
  const response = offers.replace({
    protocolVersion: 1,
    sources: [
      {
        id: "window:42",
        kind: "window",
        applicationName: "Example Game",
        bundleIdentifier: "com.example.game",
        title: "Example Game",
        width: 1920,
        height: 1080,
        preview: new Uint8Array(),
      },
    ],
  });

  assert.equal(response.sources[0].id, "opaque-1");
  assert.equal(JSON.stringify(response).includes("window:42"), false);
  assert.deepEqual(offers.consume("opaque-1"), {
    kind: "window",
    nativeId: 42,
    expectedBundleIdentifier: "com.example.game",
  });
  assert.throws(() => offers.consume("opaque-1"), /expired/);

  const replacement = offers.replace({
    protocolVersion: 1,
    sources: [
      {
        id: "display:7",
        kind: "display",
        displayIndex: 1,
        isMainDisplay: true,
        width: 2560,
        height: 1440,
        preview: new Uint8Array(),
      },
    ],
  });
  now += 2 * 60 * 1000;
  assert.throws(() => offers.consume(replacement.sources[0].id), /expired/);
});

test("new source enumeration invalidates earlier offers", () => {
  let nextToken = 0;
  const offers = new MacOSGameCaptureSourceOffers({
    createToken: () => `opaque-${++nextToken}`,
  });
  const source = {
    id: "display:7",
    kind: "display",
    displayIndex: 1,
    isMainDisplay: true,
    width: 2560,
    height: 1440,
    preview: new Uint8Array(),
  };
  const first = offers.replace({ protocolVersion: 1, sources: [source] });
  offers.replace({ protocolVersion: 1, sources: [source] });
  assert.throws(() => offers.consume(first.sources[0].id), /expired/);
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

function sourcePreviewFrame(manifest, previews = []) {
  const manifestBuffer = Buffer.from(JSON.stringify(manifest));
  const prefix = Buffer.alloc(4);
  prefix.writeUInt32BE(manifestBuffer.length);
  return Buffer.concat([prefix, manifestBuffer, ...previews]);
}
