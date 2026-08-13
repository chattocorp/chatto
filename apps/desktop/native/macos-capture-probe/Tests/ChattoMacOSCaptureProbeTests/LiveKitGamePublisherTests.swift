// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

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
