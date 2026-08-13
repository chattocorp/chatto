// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import { randomUUID } from "node:crypto";

export const gameCaptureListSourcesChannel = "chatto:game-capture:list-sources";
export const gameCaptureCancelListSourcesChannel =
  "chatto:game-capture:cancel-list-sources";
export const gameCaptureStartChannel = "chatto:game-capture:start";
export const gameCapturePublisherChannel = "chatto:game-capture:publisher";

const minimumMacOSGameCaptureMajorVersion = 15;
const sourcePreviewManifestMaximumBytes = 1024 * 1024;
const sourcePreviewMaximumBytes = 512 * 1024;
const sourcePreviewFrameMaximumBytes = 16 * 1024 * 1024;
const sourcePreviewMaximumCount = 64;
const sourceOfferLifetimeMilliseconds = 2 * 60 * 1000;

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
 * Parse the trusted macOS helper response. The temporary native IDs in this
 * result must be replaced with renderer-facing offers before crossing IPC.
 */
export function parseMacOSGameCaptureSources(output) {
  if (!Buffer.isBuffer(output) || output.length < 4) {
    throw new Error("The capture helper returned an unsupported source list.");
  }
  if (output.length > sourcePreviewFrameMaximumBytes) {
    throw new Error("The capture helper returned too much preview data.");
  }

  const manifestLength = output.readUInt32BE(0);
  if (
    manifestLength === 0 ||
    manifestLength > sourcePreviewManifestMaximumBytes ||
    4 + manifestLength > output.length
  ) {
    throw new Error("The capture helper returned an invalid source manifest.");
  }
  let response;
  try {
    response = JSON.parse(output.subarray(4, 4 + manifestLength).toString());
  } catch {
    throw new Error("The capture helper returned an invalid source manifest.");
  }
  if (
    response?.protocolVersion !== 1 ||
    !Array.isArray(response.sources) ||
    response.sources.length > sourcePreviewMaximumCount
  ) {
    throw new Error("The capture helper returned an unsupported source list.");
  }

  let previewOffset = 4 + manifestLength;
  const sources = response.sources.map((source) => {
    validateMacOSCaptureSource(source);
    const previewEnd = previewOffset + source.previewByteLength;
    if (previewEnd > output.length) {
      throw new Error("The capture helper returned truncated preview data.");
    }
    const preview = Uint8Array.from(output.subarray(previewOffset, previewEnd));
    previewOffset = previewEnd;

    if (source.kind === "display") {
      return {
        id: `display:${source.nativeID}`,
        kind: "display",
        displayIndex: source.displayIndex,
        isMainDisplay: source.isMainDisplay,
        width: source.width,
        height: source.height,
        preview,
      };
    }
    return {
      id: `window:${source.nativeID}`,
      kind: "window",
      applicationName: source.applicationName,
      bundleIdentifier: source.bundleIdentifier,
      title: source.title,
      width: source.width,
      height: source.height,
      preview,
    };
  });

  if (previewOffset !== output.length) {
    throw new Error("The capture helper returned unexpected preview data.");
  }

  return {
    protocolVersion: 1,
    sources,
  };
}

function validateMacOSCaptureSource(source) {
  if (
    !source ||
    !["display", "window"].includes(source.kind) ||
    !Number.isSafeInteger(source.nativeID) ||
    source.nativeID <= 0 ||
    !Number.isSafeInteger(source.width) ||
    source.width <= 0 ||
    source.width > 16384 ||
    !Number.isSafeInteger(source.height) ||
    source.height <= 0 ||
    source.height > 16384 ||
    !Number.isSafeInteger(source.previewByteLength) ||
    source.previewByteLength < 0 ||
    source.previewByteLength > sourcePreviewMaximumBytes
  ) {
    throw new Error("The capture helper returned an invalid capture source.");
  }
  if (
    source.kind === "display" &&
    (!Number.isSafeInteger(source.displayIndex) ||
      source.displayIndex <= 0 ||
      typeof source.isMainDisplay !== "boolean")
  ) {
    throw new Error("The capture helper returned an invalid display source.");
  }
  if (
    source.kind === "window" &&
    (typeof source.applicationName !== "string" ||
      source.applicationName.length > 4096 ||
      typeof source.bundleIdentifier !== "string" ||
      source.bundleIdentifier.length === 0 ||
      source.bundleIdentifier.length > 512 ||
      typeof source.title !== "string" ||
      source.title.length > 4096)
  ) {
    throw new Error("The capture helper returned an invalid window source.");
  }
}

/** Resolve an opaque renderer source ID at the platform boundary. */
export function parseMacOSGameCaptureSourceId(sourceId) {
  const match = /^(window|display):([1-9][0-9]*)$/.exec(sourceId);
  if (!match)
    throw new Error("The selected native screen-share source is invalid.");
  const nativeId = Number(match[2]);
  if (!Number.isSafeInteger(nativeId) || nativeId > 0xffff_ffff) {
    throw new Error("The selected native screen-share source is invalid.");
  }
  return { kind: match[1], nativeId };
}

/**
 * Issue short-lived, single-use source offers to the renderer. Native window
 * and display identifiers never cross IPC, and every new enumeration
 * invalidates the previous set.
 */
export class MacOSGameCaptureSourceOffers {
  #createToken;
  #now;
  #offers = new Map();

  constructor({ createToken = randomUUID, now = Date.now } = {}) {
    this.#createToken = createToken;
    this.#now = now;
  }

  replace(response) {
    this.#offers.clear();
    return {
      protocolVersion: response.protocolVersion,
      sources: response.sources.map((source) => {
        const nativeSource = parseMacOSGameCaptureSourceId(source.id);
        let id;
        do {
          id = this.#createToken();
        } while (this.#offers.has(id));
        if (typeof id !== "string" || id.length === 0 || id.length > 128) {
          throw new Error("The desktop host could not offer a capture source.");
        }
        this.#offers.set(id, {
          source: nativeSource,
          expectedBundleIdentifier:
            source.kind === "window" ? source.bundleIdentifier : undefined,
          expiresAt: this.#now() + sourceOfferLifetimeMilliseconds,
        });
        return { ...source, id };
      }),
    };
  }

  consume(id) {
    const offer = this.#offers.get(id);
    this.#offers.delete(id);
    if (!offer || offer.expiresAt <= this.#now()) {
      throw new Error(
        "The selected native screen-share source has expired. Choose it again.",
      );
    }
    return {
      ...offer.source,
      expectedBundleIdentifier: offer.expectedBundleIdentifier,
    };
  }

  clear() {
    this.#offers.clear();
  }
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
