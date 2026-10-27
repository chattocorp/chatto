// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import CoreVideo
import Foundation
import LiveKit
import Testing

@testable import ChattoMacOSCaptureProbe

@Test
func automaticRateControlSupportsOlderSystemsAndFallsBackOnlyOnce() {
  #expect(TunedRateControl.automatic.initialMode(vbrAvailable: true) == .vbr)
  #expect(TunedRateControl.automatic.initialMode(vbrAvailable: false) == .average)
  #expect(TunedRateControl.automatic.fallback(after: .vbr) == .average)
  #expect(TunedRateControl.automatic.fallback(after: .average) == nil)
  #expect(TunedRateControl.vbr.initialMode(vbrAvailable: false) == .vbr)
  #expect(TunedRateControl.vbr.fallback(after: .vbr) == nil)
  #expect(TunedRateControl.average.initialMode(vbrAvailable: true) == .average)
}

@Test
func tunedBudgetFollowsCongestionControlWithoutExceedingEightMbps() {
  #expect(TunedVideoBudget(kbps: 8000).averageBps == 7_200_000)
  #expect(TunedVideoBudget(kbps: 3000).averageBps == 2_700_000)
  #expect(TunedVideoBudget(kbps: 3000).ceilingBps == 3_000_000)
  #expect(TunedVideoBudget(kbps: .max).ceilingBps == 8_000_000)
  #expect(TunedVideoBudget(kbps: 8000, maximumBps: 4_000_000).averageBps == 3_600_000)
  #expect(TunedVideoBudget(kbps: 0).averageBps == 0)
  #expect(TunedVideoBudget(kbps: 0).ceilingBps == 0)
  #expect(TunedVideoBudget(kbps: 8000, rateControl: .vbr).averageBps == 6_000_000)
  #expect(TunedVideoBudget(kbps: 5000, rateControl: .vbr).averageBps == 4_500_000)
  #expect(TunedVideoBudget(kbps: 0, rateControl: .vbr).averageBps == 0)
}

@Test
func tunedFactoryCannotSilentlySelectBaselineOrAnotherCodec() {
  let factory = TunedH264Factory(diagnostics: TunedEncoderDiagnostics())
  #expect(factory.supportedCodecs.count == 1)
  #expect(factory.createEncoder(for: factory.supportedCodecs[0]) != nil)
  for profile in ["42e034", "640c1f", "private value"] {
    #expect(factory.createEncoder(for: VideoCodecInfo(name: "H264", parameters: ["profile-level-id": profile, "packetization-mode": "1"])) == nil)
  }
  #expect(factory.createEncoder(for: VideoCodecInfo(name: "H265")) == nil)
}

@Test
func tunedDimensionsKeepWidescreenDetailAndFitOtherAspectRatios() throws {
  #expect(TunedH264Encoder.encodeDimensions(width: 1920, height: 1122) == Dimensions(width: 1920, height: 1122))
  #expect(TunedH264Encoder.encodeDimensions(width: 1920, height: 1200) == Dimensions(width: 1872, height: 1170))
  #expect(TunedH264Encoder.encodeDimensions(width: 1920, height: 1920) == Dimensions(width: 1488, height: 1488))
  #expect(TunedH264Encoder.encodeDimensions(width: 0, height: 1080) == nil)
  #expect(TunedH264Encoder.encodeDimensions(width: .max, height: 1080) == nil)
  for (width, height) in [(1200, 1920), (1440, 1920), (3840, 2160), (1920, 1440)] {
    let dimensions = try #require(TunedH264Encoder.encodeDimensions(width: width, height: height))
    let w = Int(dimensions.width), h = Int(dimensions.height)
    #expect(w <= 1920 && h <= 1920 && w % 2 == 0 && h % 2 == 0)
    #expect(((w + 15) / 16) * ((h + 15) / 16) <= 8704)
    #expect(abs(Double(w) / Double(h) - Double(width) / Double(height)) < 0.01)
  }
}

@Test
func avccConversionPreservesNALUnitsAndRejectsTruncatedSamples() throws {
  #expect(try H264Payload.annexB(Data([0, 0, 0, 2, 0x65, 0x80, 0, 0, 0, 1, 0x06]), lengthBytes: 4)
    == Data([0, 0, 0, 1, 0x65, 0x80, 0, 0, 0, 1, 0x06]))
  #expect(try H264Payload.annexB(Data([1, 0x65]), lengthBytes: 1) == Data([0, 0, 0, 1, 0x65]))
  for bytes: [UInt8] in [[], [0], [0, 0, 0, 0], [0, 0, 0, 2, 0x65], [255, 255, 255, 255]] {
    #expect(throws: H264Payload.PayloadError.self) { try H264Payload.annexB(Data(bytes), lengthBytes: 4) }
  }
}

/// Each fixture owns the hardware encoder's throughput. VT callbacks still run
/// concurrently with control calls; independent 60 FPS fixtures must not compete.
@Suite(.serialized)
struct HardwareEncoderTests {
  /// CI runners do not necessarily expose a hardware encoder. Run this explicitly
  /// on a Mac before shipping encoder changes; it must fail rather than use software.
  @Test(.enabled(if: ProcessInfo.processInfo.environment["CHATTO_TEST_HARDWARE_ENCODER"] == "1"))
  func tunedHardwareEncoderPreservesTimestampsAndHonorsRateChanges() throws {
    let diagnostics = TunedEncoderDiagnostics()
    let encoder = TunedH264Encoder(diagnostics: diagnostics)
    let output = EncodedFrames()
    encoder.setCallback { output.append($0); return true }
    let settings = VideoEncoderSettings(name: "H264", dimensions: Dimensions(width: 640, height: 360),
      startBitrateKbps: 8000, maxBitrateKbps: 8000, minBitrateKbps: 0, maxFramerate: 60, qpMax: 39, mode: .screensharing)
    try #require(encoder.startEncode(with: settings, numberOfCores: 4) == .ok)
    defer { _ = encoder.releaseEncoder() }
    #expect(diagnostics.snapshot().hardware == true)
    #expect(diagnostics.snapshot().appliedAverageBps == 7_200_000)
    for index in 0..<90 {
      if index == 30 {
        #expect(encoder.setBitrate(3000, framerate: 60) == .ok)
        #expect(diagnostics.snapshot().appliedAverageBps == 2_700_000)
        #expect(diagnostics.snapshot().ceilingBps == 3_000_000)
      }
      if index == 45 { #expect(encoder.setBitrate(0, framerate: 0) == .ok) }
      if index == 46 { #expect(encoder.setBitrate(8000, framerate: 60) == .ok) }
      let frame = try makeEncoderFrame(index: index, width: index >= 70 ? 1920 : 640, height: index >= 70 ? 1122 : 360)
      #expect(encoder.encode(frame, frameTypes: [index == 60 ? .key : .delta]) == .ok)
      if index == 45 {
        #expect(output.ready.wait(timeout: .now() + 0.03) == .timedOut)
      } else {
        try #require(output.ready.wait(timeout: .now() + 3) == .success)
        let encoded = try #require(output.frames.last)
        #expect(encoded.rtpTimestamp == frame.rtpTimestamp)
        #expect(encoded.dimensions == frame.dimensions)
        #expect(encoded.qp == nil)
        if [0, 46, 60, 70].contains(index) { #expect(encoded.frameType == .key) }
        #expect(encoded.data.starts(with: [0, 0, 0, 1]))
      }
    }
    #expect(diagnostics.snapshot().framesEncoded == 89)
    #expect(diagnostics.snapshot().framesDropped == 1)
    #expect(diagnostics.snapshot().callbackRejected == 0)
    #expect(diagnostics.snapshot().lastStatus == 0)
    #expect(diagnostics.snapshot().profile == "640c2a")
    // Save an elementary stream only when explicitly requested for independent
    // decoder validation. The test frames contain no capture or user content.
    if let path = ProcessInfo.processInfo.environment["CHATTO_TEST_ENCODED_OUTPUT"] {
      try output.frames.reduce(into: Data()) { $0.append($1.data) }.write(to: URL(fileURLWithPath: path))
    }
    #expect(encoder.releaseEncoder() == .ok)
    #expect(encoder.encode(try makeEncoderFrame(index: 90), frameTypes: [.delta]) == .uninitialized)
    #expect(output.ready.wait(timeout: .now() + 0.03) == .timedOut)
    // WebRTC can register the callback after restarting the same instance.
    try #require(encoder.startEncode(with: settings, numberOfCores: 4) == .ok)
    encoder.setCallback { output.append($0); return true }
    #expect(encoder.encode(try makeEncoderFrame(index: 91), frameTypes: [.delta]) == .ok)
    try #require(output.ready.wait(timeout: .now() + 3) == .success)
    #expect(output.frames.last?.frameType == .key)
  }

  /// Feed at capture cadence: VBR may retain frames internally, so waiting for
  /// each callback before submitting the next frame would deadlock this mode.
  @Test(.enabled(if: ProcessInfo.processInfo.environment["CHATTO_TEST_HARDWARE_ENCODER"] == "1"))
  func vbrHardwareEncoderFollowsRateChangesWithABoundedQueue() async throws {
    guard #available(macOS 26, *) else { return }
    let diagnostics = TunedEncoderDiagnostics()
    let encoder = TunedH264Encoder(diagnostics: diagnostics, rateControl: .automatic)
    let output = EncodedFrames()
    encoder.setCallback { output.append($0); return true }
    let settings = VideoEncoderSettings(name: "H264", dimensions: Dimensions(width: 1920, height: 1122),
      startBitrateKbps: 0, maxBitrateKbps: 8000, minBitrateKbps: 0, maxFramerate: 60, qpMax: 39, mode: .screensharing)
    try #require(encoder.startEncode(with: settings, numberOfCores: 4) == .ok)
    defer { _ = encoder.releaseEncoder() }
    #expect(diagnostics.snapshot().hardware == true)
    #expect(diagnostics.snapshot().rateControl == "vbr")
    #expect(diagnostics.snapshot().requestedRateControl == "auto")
    #expect(diagnostics.snapshot().ceilingBps == 0)
    #expect(encoder.setBitrate(8000, framerate: 60) == .ok)
    #expect(diagnostics.snapshot().appliedAverageBps == 6_000_000)
    #expect(diagnostics.snapshot().appliedCeilingBps == 8_000_000)
    #expect(diagnostics.snapshot().vbvBufferMs == 250)
    let started = ContinuousClock.now
    for index in 0..<120 {
      try await ContinuousClock().sleep(until: started + .nanoseconds(Int64(index) * 16_666_667))
      if index == 30 {
        #expect(encoder.setBitrate(3000, framerate: 60) == .ok)
        #expect(diagnostics.snapshot().appliedAverageBps == 2_700_000)
        #expect(diagnostics.snapshot().appliedCeilingBps == 3_000_000)
      }
      if index == 50 { #expect(encoder.setBitrate(0, framerate: 0) == .ok) }
      if index == 55 { #expect(encoder.setBitrate(8000, framerate: 60) == .ok) }
      #expect(encoder.encode(try makeEncoderFrame(index: index, width: 1920, height: 1122), frameTypes: [.delta]) == .ok)
    }
    try await Task.sleep(for: .milliseconds(100))
    let frames = output.frames
    // Permit the few frames retained by VT at the tail or dropped on pause. A
    // stalled bounded queue must not pass simply because encode returned OK.
    #expect(frames.count >= 105)
    #expect(Set(frames.map(\.rtpTimestamp)).count == frames.count)
    #expect(frames.allSatisfy { $0.dimensions == settings.dimensions })
    #expect(frames.contains { $0.rtpTimestamp == 12345 + 55 * 1500 && $0.frameType == .key })
    #expect(diagnostics.snapshot().peakPendingFrames <= 6)
    #expect(diagnostics.snapshot().callbackRejected == 0)
    #expect(diagnostics.snapshot().lastStatus == 0)
    if let path = ProcessInfo.processInfo.environment["CHATTO_TEST_ENCODED_OUTPUT"] {
      try frames.reduce(into: Data()) { $0.append($1.data) }.write(to: URL(fileURLWithPath: path + ".vbr.h264"))
    }
    // A bounded queue is allowed to drop input under pressure. After release,
    // every submitted frame must be accounted for as delivered or dropped; no
    // pending callback or leaked token may remain. Timing is measured separately
    // in the paced motion benchmark, not asserted against host load here.
    #expect(encoder.releaseEncoder() == .ok)
    #expect(diagnostics.snapshot().framesEncoded == output.frames.count)
    #expect(diagnostics.snapshot().framesEncoded + diagnostics.snapshot().framesDropped == 120)
    #expect(diagnostics.snapshot().queueDrops <= diagnostics.snapshot().framesDropped)
  }

  /// The capturer limits the longest edge, so 16:10 and square windows can exceed
  /// level 4.2's frame budget even when neither edge exceeds 1920 pixels.
  @Test(.enabled(if: ProcessInfo.processInfo.environment["CHATTO_TEST_HARDWARE_ENCODER"] == "1"),
    arguments: [TunedRateControl.average, .automatic])
  func tunedHardwareEncoderFitsTallSourcesWithinTheNegotiatedLevel(rateControl: TunedRateControl) async throws {
    let diagnostics = TunedEncoderDiagnostics()
    let encoder = TunedH264Encoder(diagnostics: diagnostics, rateControl: rateControl)
    let output = EncodedFrames()
    encoder.setCallback { output.append($0); return true }
    let settings = VideoEncoderSettings(name: "H264", dimensions: Dimensions(width: 1920, height: 1200),
      startBitrateKbps: 8000, maxBitrateKbps: 8000, minBitrateKbps: 0, maxFramerate: 60, qpMax: 39, mode: .screensharing)
    defer { _ = encoder.releaseEncoder() }
    try #require(encoder.startEncode(with: settings, numberOfCores: 4) == .ok)
    let started = ContinuousClock.now
    for index in 0..<60 {
      try await ContinuousClock().sleep(until: started + .nanoseconds(Int64(index) * 16_666_667))
      #expect(encoder.encode(try makeEncoderFrame(index: index, width: 1920, height: index < 30 ? 1200 : 1920, edgeMarkers: true), frameTypes: [.delta]) == .ok)
    }
    try await Task.sleep(for: .milliseconds(100))
    let frames = output.frames
    #expect(frames.count >= 40)
    for frame in frames {
      let index = (frame.rtpTimestamp - 12345) / 1500
      #expect(frame.dimensions == (index < 30 ? Dimensions(width: 1872, height: 1170) : Dimensions(width: 1488, height: 1488)))
    }
    #expect(frames.contains { $0.dimensions.width == 1872 && $0.frameType == .key })
    #expect(frames.contains { $0.dimensions.width == 1488 && $0.frameType == .key })
    #expect(diagnostics.snapshot().lastStatus == 0)
    #expect(diagnostics.snapshot().profile == "640c2a")
    if let path = ProcessInfo.processInfo.environment["CHATTO_TEST_ENCODED_OUTPUT"] {
      try frames.reduce(into: Data()) { $0.append($1.data) }.write(to: URL(fileURLWithPath: path + ".aspect." + rateControl.rawValue + ".h264"))
    }
  }
}

private final class EncodedFrames: @unchecked Sendable {
  let ready = DispatchSemaphore(value: 0)
  private let lock = NSLock()
  private var stored: [EncodedVideoFrame] = []
  var frames: [EncodedVideoFrame] {
    lock.lock(); defer { lock.unlock() }; return stored
  }
  func append(_ frame: EncodedVideoFrame) {
    lock.lock(); stored.append(frame); lock.unlock(); ready.signal()
  }
}

private func makeEncoderFrame(index: Int, width: Int = 640, height: Int = 360, edgeMarkers: Bool = false) throws -> VideoFrame {
  var pixel: CVPixelBuffer?
  try #require(CVPixelBufferCreate(nil, width, height, kCVPixelFormatType_32BGRA,
    [kCVPixelBufferIOSurfacePropertiesKey: [:]] as CFDictionary, &pixel) == kCVReturnSuccess)
  let buffer = try #require(pixel)
  CVPixelBufferLockBaseAddress(buffer, [])
  let address = try #require(CVPixelBufferGetBaseAddress(buffer))
  memset(address, Int32(index * 2), CVPixelBufferGetBytesPerRow(buffer) * height)
  if edgeMarkers {
    // Independent decoder checks inspect these right/bottom borders to detect
    // cropping instead of scaling when source and coded dimensions differ.
    let stride = CVPixelBufferGetBytesPerRow(buffer)
    for y in 0..<height { memset(address.advanced(by: y * stride + (width - 16) * 4), 255, 16 * 4) }
    memset(address.advanced(by: (height - 16) * stride), 255, stride * 16)
  }
  CVPixelBufferUnlockBaseAddress(buffer, [])
  return VideoFrame(dimensions: Dimensions(width: Int32(width), height: Int32(height)), rotation: ._0,
    timeStampNs: Int64(index) * 16_666_667, rtpTimestamp: UInt32(12345 + index * 1500), buffer: CVPixelVideoBuffer(pixelBuffer: buffer))
}
