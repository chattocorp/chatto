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
const encodedPreviewFrameMaximumBytes = 16 * 1024 * 1024;
const encodedPreviewHeaderBytes = 16;

/** Parse length-delimited Annex-B H.264 frames from the helper's local preview pipe. */
export class EncodedPreviewFrameParser {
  #pending = Buffer.alloc(0);

  push(chunk) {
    if (!Buffer.isBuffer(chunk)) {
      throw new Error("The capture helper returned invalid preview data.");
    }
    this.#pending =
      this.#pending.length === 0 ? chunk : Buffer.concat([this.#pending, chunk]);
    const frames = [];
    for (;;) {
      if (this.#pending.length < encodedPreviewHeaderBytes) break;
      if (this.#pending.toString("ascii", 0, 4) !== "CTPV") {
        throw new Error("The capture helper returned invalid preview data.");
      }
      const encodedSize = this.#pending.readUInt32LE(4);
      const keyFrame = (encodedSize & 0x8000_0000) !== 0;
      const size = encodedSize & 0x7fff_ffff;
      if (size === 0 || size > encodedPreviewFrameMaximumBytes) {
        throw new Error("The capture helper returned invalid preview data.");
      }
      const recordSize = encodedPreviewHeaderBytes + size;
      if (this.#pending.length < recordSize) break;
      frames.push({
        timestampUs: Number(this.#pending.readBigInt64LE(8)),
        keyFrame,
        data: Uint8Array.from(
          this.#pending.subarray(encodedPreviewHeaderBytes, recordSize),
        ),
      });
      this.#pending = this.#pending.subarray(recordSize);
    }
    if (this.#pending.length > encodedPreviewFrameMaximumBytes + encodedPreviewHeaderBytes) {
      throw new Error("The capture helper returned too much preview data.");
    }
    return frames;
  }
}

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

/** Whether Windows Graphics Capture and process-loopback audio are available. */
export function supportsWindowsGameCapture(systemVersion) {
  if (typeof systemVersion !== "string") return false;
  const match = /^(\d+)\.(\d+)\.(\d+)$/.exec(systemVersion);
  if (!match) return false;
  const [major, minor, build] = match.slice(1).map(Number);
  return (
    Number.isSafeInteger(major) &&
    Number.isSafeInteger(minor) &&
    Number.isSafeInteger(build) &&
    (major > 10 || (major === 10 && build >= 19041))
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

/** Parse the Windows helper manifest and attach Electron-owned JPEG previews. */
export function parseWindowsGameCaptureSources(output, previews = new Map()) {
  if (
    !Buffer.isBuffer(output) ||
    output.length === 0 ||
    output.length > 1024 * 1024
  ) {
    throw new Error("The capture helper returned an unsupported source list.");
  }
  let response;
  try {
    response = JSON.parse(output.toString("utf8"));
  } catch {
    throw new Error("The capture helper returned an unsupported source list.");
  }
  if (
    response?.protocolVersion !== 1 ||
    !Array.isArray(response.sources) ||
    response.sources.length > sourcePreviewMaximumCount
  ) {
    throw new Error("The capture helper returned an unsupported source list.");
  }
  return {
    protocolVersion: 1,
    sources: response.sources.map((source) => {
      validateMacOSCaptureSource(source);
      const preview = previews.get(source.nativeID) ?? new Uint8Array();
      if (
        !(preview instanceof Uint8Array) ||
        preview.byteLength > sourcePreviewMaximumBytes
      ) {
        throw new Error("The capture helper returned too much preview data.");
      }
      if (source.kind === "display") {
        return {
          id: `display:${source.nativeID}`,
          kind: "display",
          displayIndex: source.displayIndex,
          isMainDisplay: source.isMainDisplay,
          width: source.width,
          height: source.height,
          preview: Uint8Array.from(preview),
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
        preview: Uint8Array.from(preview),
      };
    }),
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
  if (value?.protocolVersion !== 1) {
    throw new Error(
      "The capture helper returned an unsupported publisher status.",
    );
  }
  if (value.kind === "metrics") {
    const integerFields = [
      "submittedFrames",
      "publishedFrames",
      "droppedFrames",
      "sourceWidth",
      "sourceHeight",
      "dimensionChanges",
      "outboundStreams",
      "activeOutboundStreams",
      "framesEncoded",
      "framesSent",
      "bytesSent",
      "cpuLimitedStreams",
      "bandwidthLimitedStreams",
      "powerEfficientStreams",
    ];
    const numberFields = [
      "captureFps",
      "publishFps",
      "averageReadbackMs",
      "averageScaleMs",
      "averagePublishMs",
      "lastPublishMs",
      "minimumActiveOutboundFps",
      "maximumActiveOutboundFps",
      "targetBitrate",
      "averageEncodeMs",
    ];
    if (
      !integerFields.every(
        (field) => Number.isSafeInteger(value[field]) && value[field] >= 0,
      ) ||
      !numberFields.every(
        (field) => Number.isFinite(value[field]) && value[field] >= 0,
      ) ||
      !["wgc-window", "wgc-monitor", "dxgi-display"].includes(
        value.captureBackend,
      ) ||
      typeof value.rtcStatsAvailable !== "boolean" ||
      !validOptionalEncoderMetrics(value) ||
      !validOptionalHardwareEncoderMetrics(value) ||
      !validOptionalNetworkMetrics(value)
    ) {
      throw new Error("The capture helper returned invalid publisher metrics.");
    }
    return {
      kind: "metrics",
      submittedFrames: value.submittedFrames,
      publishedFrames: value.publishedFrames,
      droppedFrames: value.droppedFrames,
      captureFps: value.captureFps,
      publishFps: value.publishFps,
      averageReadbackMs: value.averageReadbackMs,
      averageScaleMs: value.averageScaleMs,
      averageGpuCopySubmitMs: value.averageGpuCopySubmitMs ?? 0,
      averageGpuConversionSubmitMs:
        value.averageGpuConversionSubmitMs ?? 0,
      averageEncoderSubmitMs: value.averageEncoderSubmitMs ?? 0,
      averageBitstreamWaitMs: value.averageBitstreamWaitMs ?? 0,
      averagePublishMs: value.averagePublishMs,
      averageHardwareEncodeMs: value.averageHardwareEncodeMs ?? 0,
      hardwareEncoderImplementation:
        value.hardwareEncoderImplementation ?? "",
      requestedEncoderBitrate: value.requestedEncoderBitrate ?? 0,
      appliedEncoderBitrate: value.appliedEncoderBitrate ?? 0,
      actualHardwareBitrate: value.actualHardwareBitrate ?? 0,
      encoderRateControlMode: value.encoderRateControlMode ?? 0,
      requestedEncoderFps: value.requestedEncoderFps ?? 0,
      hardwareEncodedFrames: value.hardwareEncodedFrames ?? 0,
      hardwareEncodedBytes: value.hardwareEncodedBytes ?? 0,
      hardwareKeyFrames: value.hardwareKeyFrames ?? 0,
      hardwareEncodedWidth: value.hardwareEncodedWidth ?? 0,
      hardwareEncodedHeight: value.hardwareEncodedHeight ?? 0,
      encoderResolutionChanges: value.encoderResolutionChanges ?? 0,
      lastPublishMs: value.lastPublishMs,
      sourceWidth: value.sourceWidth,
      sourceHeight: value.sourceHeight,
      dimensionChanges: value.dimensionChanges,
      captureBackend: value.captureBackend,
      rtcStatsAvailable: value.rtcStatsAvailable,
      outboundStreams: value.outboundStreams,
      activeOutboundStreams: value.activeOutboundStreams,
      minimumActiveOutboundFps: value.minimumActiveOutboundFps,
      maximumActiveOutboundFps: value.maximumActiveOutboundFps,
      framesEncoded: value.framesEncoded,
      framesSent: value.framesSent,
      bytesSent: value.bytesSent,
      retransmittedPacketsSent: value.retransmittedPacketsSent ?? 0,
      retransmittedBytesSent: value.retransmittedBytesSent ?? 0,
      nackCount: value.nackCount ?? 0,
      pliCount: value.pliCount ?? 0,
      targetBitrate: value.targetBitrate,
      averageEncodeMs: value.averageEncodeMs,
      encodedWidth: value.encodedWidth ?? 0,
      encodedHeight: value.encodedHeight ?? 0,
      averageQp: value.averageQp ?? 0,
      encoderImplementation: value.encoderImplementation ?? "",
      cpuLimitedStreams: value.cpuLimitedStreams,
      bandwidthLimitedStreams: value.bandwidthLimitedStreams,
      powerEfficientStreams: value.powerEfficientStreams,
      remoteInboundStatsAvailable:
        value.remoteInboundStatsAvailable ?? false,
      remotePacketsLost: value.remotePacketsLost ?? 0,
      remoteJitterSeconds: value.remoteJitterSeconds ?? 0,
      remoteFractionLost: value.remoteFractionLost ?? 0,
      remoteRoundTripTimeMs: value.remoteRoundTripTimeMs ?? 0,
      candidatePairStatsAvailable:
        value.candidatePairStatsAvailable ?? false,
      availableOutgoingBitrate: value.availableOutgoingBitrate ?? 0,
      currentRoundTripTimeMs: value.currentRoundTripTimeMs ?? 0,
      packetsDiscardedOnSend: value.packetsDiscardedOnSend ?? 0,
      bytesDiscardedOnSend: value.bytesDiscardedOnSend ?? 0,
    };
  }
  if (value.kind !== "started") {
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

function validOptionalEncoderMetrics(value) {
  const fields = [
    value.encodedWidth,
    value.encodedHeight,
    value.averageQp,
    value.encoderImplementation,
  ];
  if (fields.every((field) => field === undefined)) return true;
  return (
    Number.isSafeInteger(value.encodedWidth) &&
    value.encodedWidth >= 0 &&
    Number.isSafeInteger(value.encodedHeight) &&
    value.encodedHeight >= 0 &&
    Number.isFinite(value.averageQp) &&
    value.averageQp >= 0 &&
    typeof value.encoderImplementation === "string" &&
    value.encoderImplementation.length <= 256
  );
}

function validOptionalHardwareEncoderMetrics(value) {
  const fields = [
    value.averageHardwareEncodeMs,
    value.hardwareEncoderImplementation,
  ];
  if (fields.every((field) => field === undefined)) return true;
  return (
    Number.isFinite(value.averageHardwareEncodeMs) &&
    value.averageHardwareEncodeMs >= 0 &&
    (value.averageGpuCopySubmitMs === undefined ||
      (Number.isFinite(value.averageGpuCopySubmitMs) &&
        value.averageGpuCopySubmitMs >= 0)) &&
    (value.averageGpuConversionSubmitMs === undefined ||
      (Number.isFinite(value.averageGpuConversionSubmitMs) &&
        value.averageGpuConversionSubmitMs >= 0)) &&
    (value.averageEncoderSubmitMs === undefined ||
      (Number.isFinite(value.averageEncoderSubmitMs) &&
        value.averageEncoderSubmitMs >= 0)) &&
    (value.averageBitstreamWaitMs === undefined ||
      (Number.isFinite(value.averageBitstreamWaitMs) &&
        value.averageBitstreamWaitMs >= 0)) &&
    typeof value.hardwareEncoderImplementation === "string" &&
    value.hardwareEncoderImplementation.length <= 256 &&
    (value.requestedEncoderBitrate === undefined ||
      (Number.isSafeInteger(value.requestedEncoderBitrate) &&
        value.requestedEncoderBitrate >= 0)) &&
    (value.appliedEncoderBitrate === undefined ||
      (Number.isSafeInteger(value.appliedEncoderBitrate) &&
        value.appliedEncoderBitrate >= 0)) &&
    (value.actualHardwareBitrate === undefined ||
      (Number.isFinite(value.actualHardwareBitrate) &&
        value.actualHardwareBitrate >= 0)) &&
    (value.encoderRateControlMode === undefined ||
      (Number.isSafeInteger(value.encoderRateControlMode) &&
        value.encoderRateControlMode >= 0)) &&
    (value.requestedEncoderFps === undefined ||
      (Number.isFinite(value.requestedEncoderFps) &&
        value.requestedEncoderFps >= 0)) &&
    (value.hardwareEncodedFrames === undefined ||
      (Number.isSafeInteger(value.hardwareEncodedFrames) &&
        value.hardwareEncodedFrames >= 0)) &&
    (value.hardwareEncodedBytes === undefined ||
      (Number.isSafeInteger(value.hardwareEncodedBytes) &&
        value.hardwareEncodedBytes >= 0)) &&
    (value.hardwareKeyFrames === undefined ||
      (Number.isSafeInteger(value.hardwareKeyFrames) &&
        value.hardwareKeyFrames >= 0)) &&
    (value.hardwareEncodedWidth === undefined ||
      (Number.isSafeInteger(value.hardwareEncodedWidth) &&
        value.hardwareEncodedWidth >= 0)) &&
    (value.hardwareEncodedHeight === undefined ||
      (Number.isSafeInteger(value.hardwareEncodedHeight) &&
        value.hardwareEncodedHeight >= 0)) &&
    (value.encoderResolutionChanges === undefined ||
      (Number.isSafeInteger(value.encoderResolutionChanges) &&
        value.encoderResolutionChanges >= 0))
  );
}

function validOptionalNetworkMetrics(value) {
  const fields = [
    value.remoteInboundStatsAvailable,
    value.candidatePairStatsAvailable,
  ];
  if (fields.every((field) => field === undefined)) return true;
  const nonNegativeIntegers = [
    value.retransmittedPacketsSent,
    value.retransmittedBytesSent,
    value.nackCount,
    value.pliCount,
    value.packetsDiscardedOnSend,
    value.bytesDiscardedOnSend,
  ];
  const nonNegativeNumbers = [
    value.remoteJitterSeconds,
    value.remoteFractionLost,
    value.remoteRoundTripTimeMs,
    value.availableOutgoingBitrate,
    value.currentRoundTripTimeMs,
  ];
  return (
    typeof value.remoteInboundStatsAvailable === "boolean" &&
    typeof value.candidatePairStatsAvailable === "boolean" &&
    Number.isSafeInteger(value.remotePacketsLost) &&
    nonNegativeIntegers.every(
      (field) => Number.isSafeInteger(field) && field >= 0,
    ) &&
    nonNegativeNumbers.every(
      (field) => Number.isFinite(field) && field >= 0,
    )
  );
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
