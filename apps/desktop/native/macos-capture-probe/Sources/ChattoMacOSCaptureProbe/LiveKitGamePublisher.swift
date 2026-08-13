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
  /// video frames and application audio remain in the Swift/WebRTC pipeline.
  static func run(
    windowID: CGWindowID,
    frameRate: Int,
    maximumWidth: Int,
    maximumHeight: Int
  ) async throws {
    // The helper reports only its small JSON lifecycle protocol. In particular,
    // signaling URLs and credentials must never leak into application logs.
    LiveKitSDK.disableLogging()
    let lifetime = PublisherLifetime()
    let credential = try readCredential()
    let sources = try await MacOSScreenCapturer.sources(for: .window)
    guard
      let source = sources.compactMap({ $0 as? MacOSWindow }).first(where: {
        $0.windowID == windowID
      })
    else {
      throw PublisherError.sourceNotFound
    }

    try AudioManager.shared.setManualRenderingMode(true)

    let videoOptions = makeVideoPublishOptions(frameRate: frameRate)
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

    let audioTrack = LocalAudioTrack.createTrack(
      name: "game-audio",
      options: .noProcessing,
      reportStatistics: true
    )
    _ = try await room.localParticipant.publish(audioTrack: audioTrack, options: audioOptions)

    let captureOptions = ScreenShareCaptureOptions(
      dimensions: Dimensions(width: Int32(maximumWidth), height: Int32(maximumHeight)),
      fps: frameRate,
      showCursor: false,
      appAudio: true
    )
    let videoTrack = LocalVideoTrack.createMacOSScreenShareTrack(
      name: "game",
      source: source,
      options: captureOptions,
      reportStatistics: true
    )
    _ = try await room.localParticipant.publish(videoTrack: videoTrack, options: videoOptions)

    try writeStatus(
      PublisherStatus(
        kind: "started",
        message: nil,
        width: maximumWidth,
        height: maximumHeight,
        frameRate: frameRate
      )
    )

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
  /// without sending them the full stream.
  static func makeVideoPublishOptions(frameRate: Int) -> VideoPublishOptions {
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
      simulcast: true,
      screenShareSimulcastLayers: lowerLayers,
      preferredCodec: .h264,
      degradationPreference: .maintainFramerate,
      streamName: "game-capture"
    )
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
  case invalidCredential
  case liveKitDisconnected
  case sourceNotFound

  var errorDescription: String? {
    switch self {
    case .invalidCredential:
      "The native publisher received an invalid LiveKit credential."
    case .liveKitDisconnected:
      "The native publisher disconnected from LiveKit unexpectedly."
    case .sourceNotFound:
      "The selected window is no longer available. Choose it again."
    }
  }
}
