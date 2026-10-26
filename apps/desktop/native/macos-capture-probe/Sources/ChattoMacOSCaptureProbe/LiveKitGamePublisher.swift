// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import CoreGraphics
import Darwin
import Foundation
import LiveKit

private struct PublisherCredential: Decodable {
  let protocolVersion: Int
  let livekitURL: String
  let token: String
  let e2eeKey: String
  let videoCodec: String?
  let videoSimulcast: Bool?
  let videoTunedH264: Bool?
  let videoTunedRateControl: String?
}

private struct PublisherStatus: Encodable {
  let protocolVersion = 1
  let kind: String
  let message: String?
  let width: Int?
  let height: Int?
  let frameRate: Int?
}

enum LiveKitGamePublisher {
  /// Connect a dedicated, publish-only companion participant directly to
  /// LiveKit. Only credentials and lifecycle status cross Desktop IPC; native
  /// video frames and any isolated window audio remain in the Swift/WebRTC
  /// pipeline.
  static func run(
    source: CaptureOptions.Source,
    expectedWindowBundleIdentifier: String?,
    frameRate: Int,
    maximumWidth: Int,
    maximumHeight: Int
  ) async throws {
    let credential = try readCredential()
    let tunedEncoder = credential.videoTunedH264 == true ? TunedEncoderDiagnostics() : nil
    // Factory registration must precede all other SDK calls. This helper owns
    // one share, so the exclusive codec list cannot affect another connection.
    if let tunedEncoder {
      let rateControl = TunedRateControl(rawValue: credential.videoTunedRateControl ?? "average") ?? .average
      if rateControl == .vbr, #unavailable(macOS 26) { throw PublisherError.vbrUnavailable }
      tunedEncoder.update {
        $0.requestedRateControl = rateControl.rawValue
        if #available(macOS 26, *) { $0.rateControl = rateControl.initialMode(vbrAvailable: true).rawValue }
        else { $0.rateControl = rateControl.initialMode(vbrAvailable: false).rawValue }
      }
      try LiveKitSDK.set(videoEncoderFactory: TunedH264Factory(diagnostics: tunedEncoder, rateControl: rateControl), exclusive: true)
    }
    // Keep SDK logging disabled. Our allowlisted diagnostics exclude signaling
    // URLs, credentials, source names, and participant identifiers.
    LiveKitSDK.disableLogging()
    let lifetime = PublisherLifetime()
    let captureSource: MacOSScreenCaptureSource
    let capturesAudio: Bool
    let showsCursor: Bool
    switch source {
    case .window(let windowID):
      let sources = try await MacOSScreenCapturer.sources(for: .window)
      guard
        let window = sources.compactMap({ $0 as? MacOSWindow }).first(where: {
          $0.windowID == windowID
        }),
        let owningBundleIdentifier = window.owningApplication?.bundleIdentifier,
        owningBundleIdentifier == expectedWindowBundleIdentifier,
        !isChattoDesktopApplication(owningBundleIdentifier)
      else { throw PublisherError.sourceNotFound }
      captureSource = window
      capturesAudio = Self.capturesAudio(
        for: source,
        windowBundleIdentifier: owningBundleIdentifier
      )
      showsCursor = Self.showsCursor(for: source)
    case .display(let displayID):
      let sources = try await MacOSScreenCapturer.sources(for: .display)
      guard
        let display = sources.compactMap({ $0 as? MacOSDisplay }).first(where: {
          $0.displayID == displayID
        })
      else { throw PublisherError.sourceNotFound }
      captureSource = display
      // Display audio includes Chatto's remote call playback. Until the native
      // path can exclude the parent Electron app upstream, publishing it would
      // echo other participants back into the room.
      capturesAudio = Self.capturesAudio(for: source)
      showsCursor = Self.showsCursor(for: source)
    case .application:
      throw PublisherError.unsupportedSource
    }

    try AudioManager.shared.setManualRenderingMode(true)

    let requestedCodec = credential.videoCodec ?? "h264"
    let simulcast = credential.videoSimulcast ?? true
    var videoOptions = makeVideoPublishOptions(frameRate: frameRate, codec: requestedCodec == "h264" ? .h264 : .h265, simulcast: simulcast)
    PublisherDiagnostic(phase: "connecting", requestedCodec: requestedCodec, simulcast: simulcast, tunedH264: tunedEncoder != nil, tunedEncoder: tunedEncoder?.snapshot()).write()
    let audioOptions = AudioPublishOptions(
      encoding: .presetMusicHighQualityStereo,
      dtx: false,
      red: false,
      streamName: "game-capture"
    )
    let roomOptions = makeRoomOptions(
      videoOptions: videoOptions,
      audioOptions: audioOptions,
      e2eeKey: credential.e2eeKey
    )
    let room = Room(delegate: lifetime)
    try await room.connect(
      url: credential.livekitURL,
      token: credential.token,
      roomOptions: roomOptions
    )

    if capturesAudio {
      let audioTrack = await LocalAudioTrack.createTrack(
        name: "game-audio",
        options: .noProcessing,
        reportStatistics: true
      )
      _ = try await room.localParticipant.publish(audioTrack: audioTrack, options: audioOptions)
    }

    let captureOptions = ScreenShareCaptureOptions(
      dimensions: Dimensions(width: Int32(maximumWidth), height: Int32(maximumHeight)),
      fps: frameRate,
      showCursor: showsCursor,
      appAudio: capturesAudio
    )
    let videoTrack = await LocalVideoTrack.createMacOSScreenShareTrack(
      name: "game",
      source: captureSource,
      options: captureOptions,
      reportStatistics: true
    )
    do {
      _ = try await room.localParticipant.publish(videoTrack: videoTrack, options: videoOptions)
    } catch let error as LiveKitError where error.type == .codecNotSupported && requestedCodec == "h265" {
      // Older SFUs can reject HEVC before publication. Do not retry unrelated
      // failures or change encryption to make a codec work.
      PublisherDiagnostic(phase: "fallback", requestedCodec: requestedCodec, simulcast: simulcast).write()
      videoOptions = makeVideoPublishOptions(frameRate: frameRate, codec: .h264, simulcast: simulcast)
      _ = try await room.localParticipant.publish(videoTrack: videoTrack, options: videoOptions)
    }

    try writeStatus(
      PublisherStatus(
        kind: "started",
        message: nil,
        width: maximumWidth,
        height: maximumHeight,
        frameRate: frameRate
      )
    )

    let diagnostics = Task {
      await PublisherDiagnostics.monitor(
        track: videoTrack, requestedCodec: requestedCodec, simulcast: simulcast,
        maintainsResolution: videoOptions.degradationPreference == .maintainResolution,
        tunedEncoder: tunedEncoder
      )
    }
    defer { diagnostics.cancel() }

    // Desktop owns process lifetime. Convert SIGTERM into a graceful LiveKit
    // disconnect so receivers see the companion publisher disappear before
    // the helper acknowledges completion by exiting.
    switch await lifetime.wait() {
    case .terminationRequested:
      await room.disconnect()
    case .roomDisconnected:
      throw PublisherError.liveKitDisconnected
    }
  }

  /// Build the gaming-oriented screen-share encodings. The full-resolution
  /// layer preserves the original 1920-pixel, 60 fps ceiling, while explicit
  /// lower layers let LiveKit serve thumbnails and constrained subscribers
  /// without sending them the full stream. The desktop default removes
  /// lower layers while preserving the full layer's bitrate and frame-rate limits.
  /// It prioritizes resolution so adaptation can reduce FPS instead of detail.
  static func makeVideoPublishOptions(frameRate: Int, codec: VideoCodec = .h264, simulcast: Bool = true) -> VideoPublishOptions {
    let lowerLayers = [
      VideoParameters(
        dimensions: .h360_169,
        encoding: VideoEncoding(maxBitrate: 1_000_000, maxFps: min(frameRate, 30))
      ),
      VideoParameters(
        dimensions: .h720_169,
        encoding: VideoEncoding(maxBitrate: 4_000_000, maxFps: min(frameRate, 60))
      ),
    ]
    return VideoPublishOptions(
      screenShareEncoding: VideoEncoding(maxBitrate: 8_000_000, maxFps: frameRate),
      simulcast: simulcast,
      screenShareSimulcastLayers: simulcast ? lowerLayers : [],
      preferredCodec: codec,
      degradationPreference: simulcast ? .maintainFramerate : .maintainResolution,
      streamName: "game-capture"
    )
  }

  /// Window capture isolates its owning application's audio. Display capture
  /// is video-only because it would otherwise include remote call playback.
  static func capturesAudio(
    for source: CaptureOptions.Source,
    windowBundleIdentifier: String? = nil
  ) -> Bool {
    if case .window = source {
      return !isChattoDesktopApplication(windowBundleIdentifier)
    }
    return false
  }

  static func showsCursor(for source: CaptureOptions.Source) -> Bool {
    if case .display = source { return true }
    return false
  }

  /// Dynacast belongs to the native companion connection. Enabling it on the
  /// frontend's separate LiveKit room cannot pause this publisher's layers.
  static func makeRoomOptions(
    videoOptions: VideoPublishOptions,
    audioOptions: AudioPublishOptions,
    e2eeKey: String
  ) -> RoomOptions {
    RoomOptions(
      defaultVideoPublishOptions: videoOptions,
      defaultAudioPublishOptions: audioOptions,
      dynacast: true,
      encryptionOptions: .sharedKey(e2eeKey)
    )
  }

  private static func readCredential() throws -> PublisherCredential {
    let data = FileHandle.standardInput.readDataToEndOfFile()
    guard !data.isEmpty, data.count <= 128 * 1024 else {
      throw PublisherError.invalidCredential
    }
    let credential = try JSONDecoder().decode(PublisherCredential.self, from: data)
    guard
      credential.protocolVersion == 1,
      let url = URL(string: credential.livekitURL),
      ["ws", "wss", "http", "https"].contains(url.scheme?.lowercased() ?? ""),
      !credential.token.isEmpty,
      !credential.e2eeKey.isEmpty
    else {
      throw PublisherError.invalidCredential
    }
    guard credential.videoCodec == nil || ["h264", "h265"].contains(credential.videoCodec!) else {
      throw PublisherError.invalidCredential
    }
    if credential.videoTunedH264 == true {
      guard credential.videoCodec == "h264", credential.videoSimulcast == false else {
        throw PublisherError.invalidCredential
      }
    }
    if let mode = credential.videoTunedRateControl {
      guard credential.videoTunedH264 == true, TunedRateControl(rawValue: mode) != nil else {
        throw PublisherError.invalidCredential
      }
    }
    return credential
  }

  private static func writeStatus(_ status: PublisherStatus) throws {
    let data = try JSONEncoder().encode(status)
    FileHandle.standardOutput.write(data)
    FileHandle.standardOutput.write(Data("\n".utf8))
  }
}

private final class PublisherLifetime: NSObject, RoomDelegate, @unchecked Sendable {
  enum StopReason: Sendable {
    case terminationRequested
    case roomDisconnected
  }

  private let source: DispatchSourceSignal
  private let stream: AsyncStream<StopReason>
  private let continuation: AsyncStream<StopReason>.Continuation

  override init() {
    Darwin.signal(SIGTERM, SIG_IGN)
    let pair = AsyncStream<StopReason>.makeStream()
    stream = pair.stream
    continuation = pair.continuation
    source = DispatchSource.makeSignalSource(signal: SIGTERM, queue: .main)
    source.setEventHandler {
      pair.continuation.yield(.terminationRequested)
      pair.continuation.finish()
    }
    super.init()
    source.resume()
  }

  func wait() async -> StopReason {
    for await reason in stream { return reason }
    return .terminationRequested
  }

  func room(_ room: Room, didDisconnectWithError error: LiveKitError?) {
    _ = room
    _ = error
    continuation.yield(.roomDisconnected)
    continuation.finish()
  }

  deinit {
    source.cancel()
  }
}

private enum PublisherError: LocalizedError {
  case vbrUnavailable
  case invalidCredential
  case liveKitDisconnected
  case sourceNotFound
  case unsupportedSource

  var errorDescription: String? {
    switch self {
    case .vbrUnavailable:
      "Tuned H.264 VBR requires macOS 26 or later. Select another Diagnostics mode."
    case .invalidCredential:
      "The native publisher received an invalid LiveKit credential."
    case .liveKitDisconnected:
      "The native publisher disconnected from LiveKit unexpectedly."
    case .sourceNotFound:
      "The selected share source is no longer available. Choose it again."
    case .unsupportedSource:
      "The selected capture source is not supported for publishing."
    }
  }
}
