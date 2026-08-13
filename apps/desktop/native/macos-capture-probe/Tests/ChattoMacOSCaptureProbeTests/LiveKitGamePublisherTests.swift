// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import Foundation
import LiveKit
import Testing

@testable import ChattoMacOSCaptureProbe

@Test
func gameVideoOptionsPublishThreeGamingLayers() throws {
  let options = LiveKitGamePublisher.makeVideoPublishOptions(frameRate: 60)

  #expect(options.simulcast)
  #expect(options.preferredCodec == .h264)
  #expect(options.degradationPreference == .maintainFramerate)
  #expect(options.streamName == "game-capture")

  let full = try #require(options.screenShareEncoding)
  #expect(full.maxBitrate == 8_000_000)
  #expect(full.maxFps == 60)

  #expect(options.screenShareSimulcastLayers.count == 2)
  let low = options.screenShareSimulcastLayers[0]
  #expect(low.dimensions.width == 640)
  #expect(low.dimensions.height == 360)
  #expect(low.encoding.maxBitrate == 1_000_000)
  #expect(low.encoding.maxFps == 30)

  let middle = options.screenShareSimulcastLayers[1]
  #expect(middle.dimensions.width == 1280)
  #expect(middle.dimensions.height == 720)
  #expect(middle.encoding.maxBitrate == 4_000_000)
  #expect(middle.encoding.maxFps == 60)
}

@Test
func lowerLayersDoNotExceedRequestedFrameRate() {
  let options = LiveKitGamePublisher.makeVideoPublishOptions(frameRate: 24)

  #expect(options.screenShareEncoding?.maxFps == 24)
  #expect(options.screenShareSimulcastLayers[0].encoding.maxFps == 24)
  #expect(options.screenShareSimulcastLayers[1].encoding.maxFps == 24)
}

@Test
func nativePublisherRoomEnablesDynacast() {
  let videoOptions = LiveKitGamePublisher.makeVideoPublishOptions(frameRate: 60)
  let audioOptions = AudioPublishOptions()
  let roomOptions = LiveKitGamePublisher.makeRoomOptions(
    videoOptions: videoOptions,
    audioOptions: audioOptions,
    e2eeKey: "test-key"
  )

  #expect(roomOptions.dynacast)
  #expect(roomOptions.defaultVideoPublishOptions == videoOptions)
  #expect(roomOptions.defaultAudioPublishOptions == audioOptions)
}

@Test
func nativeSourceMediaPolicyAvoidsDisplayAudioFeedback() {
  #expect(
    LiveKitGamePublisher.capturesAudio(
      for: .window(42),
      windowBundleIdentifier: "com.example.game"
    )
  )
  #expect(
    !LiveKitGamePublisher.capturesAudio(
      for: .window(42),
      windowBundleIdentifier: chattoDesktopApplicationBundleIdentifier
    )
  )
  #expect(!LiveKitGamePublisher.showsCursor(for: .window(42)))
  #expect(!LiveKitGamePublisher.capturesAudio(for: .display(7)))
  #expect(LiveKitGamePublisher.showsCursor(for: .display(7)))
}

@Test
func sourcePreviewFramesKeepBinaryPreviewsOutOfJSON() throws {
  let first = Data([0xff, 0xd8, 0xff, 0x01])
  let second = Data([0xff, 0xd8, 0xff, 0x02, 0x03])
  let manifest = SourcePreviewManifest(sources: [
    SourcePreviewRecord(
      kind: .display,
      nativeID: 7,
      applicationName: nil,
      bundleIdentifier: nil,
      title: "",
      width: 2560,
      height: 1440,
      displayIndex: 1,
      isMainDisplay: true,
      previewByteLength: first.count
    ),
    SourcePreviewRecord(
      kind: .window,
      nativeID: 42,
      applicationName: "Moonring",
      bundleIdentifier: "com.example.moonring",
      title: "Moonring",
      width: 1920,
      height: 1080,
      displayIndex: nil,
      isMainDisplay: nil,
      previewByteLength: second.count
    ),
  ])

  let frame = try SourcePreviewProtocol.encode(manifest: manifest, previews: [first, second])
  let manifestLength = frame.prefix(4).reduce(UInt32(0)) { ($0 << 8) | UInt32($1) }
  let manifestStart = 4
  let manifestEnd = manifestStart + Int(manifestLength)
  let decoded = try JSONDecoder().decode(
    SourcePreviewManifest.self,
    from: frame[manifestStart..<manifestEnd]
  )

  #expect(decoded == manifest)
  #expect(frame[manifestEnd..<(manifestEnd + first.count)] == first)
  #expect(frame[(manifestEnd + first.count)...] == second)
}
