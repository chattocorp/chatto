// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

export const gameCaptureListSourcesChannel = "chatto:game-capture:list-sources";
export const gameCaptureStartChannel = "chatto:game-capture:start";
export const gameCapturePublisherChannel = "chatto:game-capture:publisher";

const minimumMacOSGameCaptureMajorVersion = 15;

/** Whether the host macOS version can launch the bundled capture helper. */
export function supportsMacOSGameCapture(systemVersion) {
  if (typeof systemVersion !== "string") return false;
  const match = /^([0-9]+)(?:\.[0-9]+){0,2}$/.exec(systemVersion);
  if (!match) return false;
  const majorVersion = Number(match[1]);
  return (
    Number.isSafeInteger(majorVersion) &&
    majorVersion >= minimumMacOSGameCaptureMajorVersion
  );
}

/**
 * Convert the macOS helper response into the platform-neutral source contract
 * exposed to the bundled frontend. Source IDs are temporary and opaque to the
 * renderer; only the desktop host interprets them.
 */
export function parseMacOSGameCaptureSources(output) {
  const response = JSON.parse(output);
  if (response?.protocolVersion !== 1 || !Array.isArray(response.windows)) {
    throw new Error("The capture helper returned an unsupported source list.");
  }

  return {
    protocolVersion: 1,
    sources: response.windows.map((window) => {
      if (
        !Number.isSafeInteger(window.windowID) ||
        window.windowID <= 0 ||
        typeof window.applicationName !== "string" ||
        typeof window.bundleIdentifier !== "string" ||
        window.bundleIdentifier.length === 0 ||
        typeof window.title !== "string" ||
        !Number.isSafeInteger(window.width) ||
        window.width <= 0 ||
        !Number.isSafeInteger(window.height) ||
        window.height <= 0
      ) {
        throw new Error(
          "The capture helper returned an invalid window source.",
        );
      }

      return {
        id: `window:${window.windowID}`,
        kind: "window",
        applicationName: window.applicationName,
        bundleIdentifier: window.bundleIdentifier,
        title: window.title,
        width: window.width,
        height: window.height,
      };
    }),
  };
}

/** Resolve an opaque renderer source ID at the platform boundary. */
export function parseMacOSGameCaptureSourceId(sourceId) {
  const match = /^window:([1-9][0-9]*)$/.exec(sourceId);
  if (!match) throw new Error("The selected game-capture source is invalid.");
  const windowId = Number(match[1]);
  if (!Number.isSafeInteger(windowId) || windowId > 0xffff_ffff) {
    throw new Error("The selected game-capture source is invalid.");
  }
  return windowId;
}

/** Validate a renderer request without ever logging or returning credentials. */
export function parseGameCapturePublisherRequest(request) {
  if (!request || typeof request !== "object") {
    throw new Error("The game-capture publisher request is invalid.");
  }
  const sourceId = validString(request.sourceId, 128);
  const livekitUrl = validString(request.livekitUrl, 2048);
  const token = validString(request.token, 64 * 1024);
  const e2eeKey = validString(request.e2eeKey, 16 * 1024);
  let url;
  try {
    url = new URL(livekitUrl);
  } catch {
    throw new Error("The game-capture LiveKit URL is invalid.");
  }
  if (!["ws:", "wss:", "http:", "https:"].includes(url.protocol)) {
    throw new Error("The game-capture LiveKit URL is invalid.");
  }
  return { sourceId, livekitUrl, token, e2eeKey };
}

/** Parse one lifecycle status line emitted by the native helper. */
export function parseGameCapturePublisherStatus(line) {
  const value = JSON.parse(line);
  if (value?.protocolVersion !== 1 || value?.kind !== "started") {
    throw new Error(
      "The capture helper returned an unsupported publisher status.",
    );
  }
  if (
    !Number.isSafeInteger(value.width) ||
    value.width <= 0 ||
    !Number.isSafeInteger(value.height) ||
    value.height <= 0 ||
    !Number.isSafeInteger(value.frameRate) ||
    value.frameRate <= 0
  ) {
    throw new Error("The capture helper returned an invalid publisher status.");
  }
  return {
    kind: "started",
    width: value.width,
    height: value.height,
    frameRate: value.frameRate,
  };
}

function validString(value, maximumLength) {
  if (
    typeof value !== "string" ||
    value.length === 0 ||
    value.length > maximumLength
  ) {
    throw new Error("The game-capture publisher request is invalid.");
  }
  return value;
}
