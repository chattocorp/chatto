// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import CoreMedia
import CoreVideo
import Foundation
import LiveKit
import VideoToolbox

enum TunedRateControl: String, Sendable {
  case automatic = "auto"
  case average
  case vbr

  /// Explicit diagnostic modes must not silently change the experiment.
  func initialMode(vbrAvailable: Bool) -> Self {
    self == .automatic ? (vbrAvailable ? .vbr : .average) : self
  }

  func fallback(after mode: Self) -> Self? {
    self == .automatic && mode == .vbr ? .average : nil
  }
}

/// Documented VT property names keep this helper buildable with pre-26 SDKs.
/// VBR use is OS-gated, and unsupported hardware properties fail explicitly.
private enum VBRProperty {
  static var target: CFString { "VariableBitRate" as CFString }
  static var maximum: CFString { "VBVMaxBitRate" as CFString }
  static var duration: CFString { "VBVBufferDuration" as CFString }
  static var initialDelay: CFString { "VBVInitialDelayPercentage" as CFString }
}

/// The network allowance bounds the rate-control maximum: a five-second data
/// ceiling in average mode, or a VBV maximum in VBR mode. Targets stay below it.
struct TunedVideoBudget: Equatable {
  let ceilingBps: Int
  let rateControl: TunedRateControl
  var averageBps: Int { min(ceilingBps * 9 / 10, rateControl == .vbr ? 6_000_000 : 8_000_000) }
  init(kbps: UInt32, maximumBps: Int = 8_000_000, rateControl: TunedRateControl = .average) {
    self.rateControl = rateControl
    ceilingBps = min(Int(kbps) * 1000, max(0, min(maximumBps, 8_000_000)))
  }
}

struct TunedEncoderSnapshot: Encodable {
  var requestedRateControl = "average"
  var rateControl = "average"
  /// Retain the failed VBR status while reporting the working fallback separately.
  var fallbackStatus: Int32?
  var appliedCeilingBps: Int?
  var vbvBufferMs: Int?
  /// WebRTC's latest video allocation, before the configured maximum is applied.
  var requestedBps = 0
  var averageBps = 0
  var ceilingBps = 0
  var appliedAverageBps: Int?
  var hardware: Bool?
  var profile: String?
  var framesEncoded = 0
  var framesDropped = 0
  var queueDrops = 0
  var peakPendingFrames = 0
  var callbackRejected = 0
  var lastStatus: Int32 = 0
  var operation = "idle"
}

/// One helper process owns one experiment. This lock protects numeric and
/// allowlisted diagnostic fields shared by control, VT callbacks, and monitoring.
final class TunedEncoderDiagnostics: @unchecked Sendable {
  private let lock = NSLock()
  private var value = TunedEncoderSnapshot()
  func update(_ body: (inout TunedEncoderSnapshot) -> Void) {
    lock.lock()
    defer { lock.unlock() }
    body(&value)
  }
  func snapshot() -> TunedEncoderSnapshot {
    lock.lock()
    defer { lock.unlock() }
    return value
  }
}

struct TunedH264Factory: VideoEncoderFactory {
  let diagnostics: TunedEncoderDiagnostics
  var rateControl: TunedRateControl = .average
  var supportedCodecs: [VideoCodecInfo] {
    [VideoCodecInfo(name: "H264", parameters: [
      "profile-level-id": "640c2a", "packetization-mode": "1", "level-asymmetry-allowed": "1",
    ])]
  }
  func createEncoder(for codec: VideoCodecInfo) -> (any VideoEncoder)? {
    guard codec.name.uppercased() == "H264", codec.parameters["packetization-mode"] == "1",
      let profile = codec.parameters["profile-level-id"]?.lowercased(),
      profile.count == 6, profile.hasPrefix("640c"), let level = UInt8(profile.suffix(2), radix: 16),
      TunedH264Encoder.profile(for: level) != nil else { return nil }
    return TunedH264Encoder(level: level, diagnostics: diagnostics, rateControl: rateControl)
  }
}

/// WebRTC serializes lifecycle/encode/rate calls. VT callbacks use a separate
/// lock for pending frames and callback ownership. No lock is held when calling
/// WebRTC. CompleteFrames + Invalidate drain VT before release returns.
final class TunedH264Encoder: VideoEncoder, @unchecked Sendable {
  let implementationName = "ChattoVideoToolboxH264"
  let supportsNativeHandle = false
  private let level: UInt8
  private let diagnostics: TunedEncoderDiagnostics
  private let requestedRateControl: TunedRateControl
  private var rateControl: TunedRateControl
  private var session: VTCompressionSession?
  private var width = 0
  private var height = 0
  private var fps = 60
  private var maximumBps = 8_000_000
  private var appliedVbrCeiling: Int?
  private var budget = TunedVideoBudget(kbps: 0)
  private var needsKeyFrame = true
  private var sequence = 0
  private let lock = NSLock()
  private var callback: VideoEncoderCallback?
  private var pending: [Int: UInt32] = [:]

  init(level: UInt8 = 0x2a, diagnostics: TunedEncoderDiagnostics, rateControl: TunedRateControl = .average) {
    self.level = level
    self.diagnostics = diagnostics
    requestedRateControl = rateControl
    if #available(macOS 26, *) {
      self.rateControl = rateControl.initialMode(vbrAvailable: true)
    } else {
      self.rateControl = rateControl.initialMode(vbrAvailable: false)
    }
  }

  // Reject low negotiated levels rather than emitting 1080p60 above the
  // receiver's stated capability. The source is bounded to a 1920-pixel edge.
  static func profile(for level: UInt8) -> CFString? {
    switch level {
    // VT exposes Constrained High only with automatic level selection. Bound
    // dimensions/FPS/rate below level 4.2 and check the emitted SPS before send.
    case 0x2a, 0x32, 0x33, 0x34: kVTProfileLevel_H264_ConstrainedHigh_AutoLevel
    default: nil
    }
  }

  /// Keep the tested size when it fits level 4.2 at 60 FPS. Other aspect ratios
  /// can exceed its 8704-macroblock frame limit despite a 1920-pixel edge.
  /// Reduce both edges together, rounding down to even pixels for 4:2:0 video.
  static func encodeDimensions(width: Int, height: Int) -> Dimensions? {
    guard width >= 2, height >= 2, width <= Int(Int32.max), height <= Int(Int32.max) else { return nil }
    func fits(_ w: Int, _ h: Int) -> Bool {
      w <= 1920 && h <= 1920 && ((w + 15) / 16) * ((h + 15) / 16) <= 8704
    }
    if fits(width, height) { return Dimensions(width: Int32(width), height: Int32(height)) }
    let sourceEdge = max(width, height)
    for edge in stride(from: min(sourceEdge, 1920) / 2 * 2, through: 2, by: -2) {
      let w = max(2, width * edge / sourceEdge / 2 * 2)
      let h = max(2, height * edge / sourceEdge / 2 * 2)
      if fits(w, h) { return Dimensions(width: Int32(w), height: Int32(h)) }
    }
    return nil
  }

  func setCallback(_ callback: VideoEncoderCallback?) {
    lock.lock()
    self.callback = callback
    lock.unlock()
  }

  func startEncode(with settings: VideoEncoderSettings, numberOfCores: Int) -> VideoEncoderStatus {
    closeSession()
    guard settings.name.uppercased() == "H264", settings.dimensions.width > 0,
      settings.dimensions.height > 0 else { return .invalidParameter }
    width = Int(settings.dimensions.width)
    height = Int(settings.dimensions.height)
    fps = max(1, min(60, Int(settings.maxFramerate)))
    maximumBps = settings.maxBitrateKbps > 0 ? min(8_000_000, Int(settings.maxBitrateKbps) * 1000) : 8_000_000
    budget = TunedVideoBudget(kbps: settings.startBitrateKbps, maximumBps: maximumBps, rateControl: rateControl)
    diagnostics.update {
      $0.requestedBps = Int(settings.startBitrateKbps) * 1000
      $0.requestedRateControl = requestedRateControl.rawValue
      $0.rateControl = rateControl.rawValue
    }
    return createSession() ? .ok : .encoderFailure
  }

  private func createSession() -> Bool {
    if createConfiguredSession() { return true }
    guard let fallback = requestedRateControl.fallback(after: rateControl) else { return false }
    closeSession()
    rateControl = fallback
    budget = TunedVideoBudget(kbps: UInt32(budget.ceilingBps / 1000), maximumBps: maximumBps, rateControl: fallback)
    diagnostics.update {
      $0.fallbackStatus = $0.lastStatus
      $0.lastStatus = 0
      $0.rateControl = fallback.rawValue
      $0.appliedAverageBps = nil
      $0.appliedCeilingBps = nil
      $0.vbvBufferMs = nil
    }
    return createConfiguredSession()
  }

  /// A failed VBR setup gets one fresh average-rate session in automatic mode.
  /// The fallback stays active for this encoder, including dimension changes.
  private func createConfiguredSession() -> Bool {
    guard let profile = Self.profile(for: level),
      let dimensions = Self.encodeDimensions(width: width, height: height) else { return false }
    var created: VTCompressionSession?
    // VideoToolbox scales source buffers to this coded size. Keep width/height
    // as the input size so each frame does not recreate the scaled session.
    let status = VTCompressionSessionCreate(
      allocator: nil, width: dimensions.width, height: dimensions.height, codecType: kCMVideoCodecType_H264,
      encoderSpecification: [kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder: true] as CFDictionary,
      imageBufferAttributes: nil, compressedDataAllocator: nil,
      outputCallback: { context, source, status, flags, sample in
        guard let context, let source else { return }
        Unmanaged<TunedH264Encoder>.fromOpaque(context).takeUnretainedValue().receive(
          sequence: Int(bitPattern: source), status: status, flags: flags, sample: sample
        )
      }, refcon: Unmanaged.passUnretained(self).toOpaque(), compressionSessionOut: &created
    )
    guard record(status, "create"), let created else { return false }
    session = created
    let properties: [(CFString, CFTypeRef)] = [
      (kVTCompressionPropertyKey_RealTime, kCFBooleanTrue),
      (kVTCompressionPropertyKey_ProfileLevel, profile),
      (kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse),
      (kVTCompressionPropertyKey_ExpectedFrameRate, fps as CFNumber),
      (kVTCompressionPropertyKey_MaxKeyFrameInterval, 7200 as CFNumber),
      (kVTCompressionPropertyKey_MaxKeyFrameIntervalDuration, 240 as CFNumber),
      (kVTCompressionPropertyKey_PrioritizeEncodingSpeedOverQuality, kCFBooleanTrue),
      (kVTCompressionPropertyKey_MaxAllowedFrameQP, 39 as CFNumber),
    ]
    for (key, value) in properties {
      guard record(VTSessionSetProperty(created, key: key, value: value), key as String) else { closeSession(); return false }
    }
    guard applyBudget(initializing: true) else { closeSession(); return false }
    if rateControl == .vbr {
      guard #available(macOS 26, *) else { _ = record(-12900, "VariableBitRate"); closeSession(); return false }
      guard record(VTSessionSetProperty(created, key: VBRProperty.duration, value: 0.25 as CFNumber), "VBVBufferDuration"),
        record(VTSessionSetProperty(created, key: VBRProperty.initialDelay, value: 0 as CFNumber), "VBVInitialDelayPercentage") else { closeSession(); return false }
      var actual: Unmanaged<CFTypeRef>?
      let status = VTSessionCopyProperty(created, key: VBRProperty.duration, allocator: nil, valueOut: &actual)
      let seconds = (actual?.takeRetainedValue() as? NSNumber)?.doubleValue
      diagnostics.update { $0.vbvBufferMs = seconds.map { Int(($0 * 1000).rounded()) } }
      guard record(status, "VBVBufferDuration") else { closeSession(); return false }
    }
    guard record(VTCompressionSessionPrepareToEncodeFrames(created), "prepare") else { closeSession(); return false }
    var hardware: Unmanaged<CFTypeRef>?
    let read = VTSessionCopyProperty(created, key: kVTCompressionPropertyKey_UsingHardwareAcceleratedVideoEncoder, allocator: nil, valueOut: &hardware)
    let accelerated = (hardware?.takeRetainedValue() as? NSNumber)?.boolValue
    diagnostics.update { $0.hardware = accelerated }
    guard record(read, "hardware"), accelerated == true else { closeSession(); return false }
    needsKeyFrame = true
    return true
  }

  func setBitrate(_ bitrateKbps: UInt32, framerate: UInt32) -> VideoEncoderStatus {
    budget = TunedVideoBudget(kbps: bitrateKbps, maximumBps: maximumBps, rateControl: rateControl)
    diagnostics.update { $0.requestedBps = Int(bitrateKbps) * 1000 }
    if framerate > 0 { fps = min(60, Int(framerate)) }
    guard let session else { return .uninitialized }
    guard record(VTSessionSetProperty(session, key: kVTCompressionPropertyKey_ExpectedFrameRate, value: fps as CFNumber), "ExpectedFrameRate"),
      applyBudget() else { return .encoderFailure }
    return .ok
  }

  private func applyBudget(initializing: Bool = false) -> Bool {
    diagnostics.update { $0.averageBps = budget.averageBps; $0.ceilingBps = budget.ceilingBps }
    guard let session else { return false }
    // VT interprets zero as automatic, not paused. Leave properties alone and
    // drop input/output while paused; request a keyframe when encoding resumes.
    var configured = budget
    if budget.ceilingBps == 0 {
      needsKeyFrame = true
      // Select VBR before preparing a session even if WebRTC starts paused.
      // No frames enter VT until the actual network allowance becomes positive.
      guard initializing && rateControl == .vbr else { return true }
      configured = TunedVideoBudget(kbps: UInt32(maximumBps / 1000), maximumBps: maximumBps, rateControl: rateControl)
    }
    if rateControl == .vbr {
      guard #available(macOS 26, *) else { return record(-12900, "VariableBitRate") }
      // These replace AverageBitRate/DataRateLimits. The VBV maximum follows
      // congestion control; it is not an instantaneous packet-rate guarantee.
      // Increase the maximum before the target; decrease the target before
      // the maximum. Never transiently request a target above the active cap.
      let raisesCeiling = appliedVbrCeiling.map { configured.ceilingBps > $0 } ?? false
      func setCeiling() -> Bool {
        record(VTSessionSetProperty(session, key: VBRProperty.maximum, value: configured.ceilingBps as CFNumber), "VBVMaxBitRate")
      }
      if raisesCeiling && !setCeiling() { return false }
      guard record(VTSessionSetProperty(session, key: VBRProperty.target, value: configured.averageBps as CFNumber), "VariableBitRate") else { return false }
      if !raisesCeiling && !setCeiling() { return false }
      appliedVbrCeiling = configured.ceilingBps
      var target: Unmanaged<CFTypeRef>?
      var ceiling: Unmanaged<CFTypeRef>?
      let targetStatus = VTSessionCopyProperty(session, key: VBRProperty.target, allocator: nil, valueOut: &target)
      let ceilingStatus = VTSessionCopyProperty(session, key: VBRProperty.maximum, allocator: nil, valueOut: &ceiling)
      let appliedTarget = (target?.takeRetainedValue() as? NSNumber)?.intValue
      let appliedCeiling = (ceiling?.takeRetainedValue() as? NSNumber)?.intValue
      diagnostics.update { $0.appliedAverageBps = appliedTarget; $0.appliedCeilingBps = appliedCeiling }
      return record(targetStatus, "VariableBitRate") && record(ceilingStatus, "VBVMaxBitRate")
    }
    let limits = [budget.ceilingBps * 10 / 8, 1, budget.ceilingBps * 5 / 8, 5] as CFArray
    guard record(VTSessionSetProperty(session, key: kVTCompressionPropertyKey_DataRateLimits, value: limits), "DataRateLimits"),
      record(VTSessionSetProperty(session, key: kVTCompressionPropertyKey_AverageBitRate, value: budget.averageBps as CFNumber), "AverageBitRate") else { return false }
    var actual: Unmanaged<CFTypeRef>?
    let status = VTSessionCopyProperty(session, key: kVTCompressionPropertyKey_AverageBitRate, allocator: nil, valueOut: &actual)
    let applied = (actual?.takeRetainedValue() as? NSNumber)?.intValue
    diagnostics.update { $0.appliedAverageBps = applied }
    return record(status, "readAverageBitRate")
  }

  func encode(_ frame: VideoFrame, frameTypes: [EncodedVideoFrame.FrameType]) -> VideoEncoderStatus {
    guard session != nil else { return .uninitialized }
    guard frameTypes.count <= 1 else { return .simulcastParametersNotSupported }
    guard budget.ceilingBps > 0, !frameTypes.contains(.empty) else { dropped(); return .ok }
    var buffer = (frame.buffer as? CVPixelVideoBuffer)?.pixelBuffer
    if buffer == nil || CVPixelBufferGetWidth(buffer!) != Int(frame.dimensions.width) || CVPixelBufferGetHeight(buffer!) != Int(frame.dimensions.height) {
      buffer = frame.toI420()?.toPixelBuffer()
    }
    guard let buffer else { return .invalidParameter }
    if width != CVPixelBufferGetWidth(buffer) || height != CVPixelBufferGetHeight(buffer) {
      closeSession()
      width = CVPixelBufferGetWidth(buffer)
      height = CVPixelBufferGetHeight(buffer)
      guard createSession() else { return .encoderFailure }
    }
    lock.lock()
    // VBR retains frames for rate control. Six frames bound its input queue to
    // 100 ms at 60 FPS while allowing the measured 22–34 ms callback latency.
    let maximumPending = rateControl == .vbr ? 6 : 3
    guard pending.count < maximumPending else {
      lock.unlock()
      diagnostics.update { $0.queueDrops += 1 }
      dropped()
      return .ok
    }
    sequence += 1
    let identifier = sequence
    pending[identifier] = frame.rtpTimestamp
    let queued = pending.count
    lock.unlock()
    diagnostics.update { $0.peakPendingFrames = max($0.peakPendingFrames, queued) }
    let forceKey = needsKeyFrame || frameTypes.contains(.key)
    var infoFlags: VTEncodeInfoFlags = []
    let status = VTCompressionSessionEncodeFrame(
      session!, imageBuffer: buffer, presentationTimeStamp: CMTime(value: frame.timeStampNs, timescale: 1_000_000_000),
      duration: CMTime(value: 1, timescale: Int32(fps)),
      frameProperties: forceKey ? [kVTEncodeFrameOptionKey_ForceKeyFrame: true] as CFDictionary : nil,
      sourceFrameRefcon: UnsafeMutableRawPointer(bitPattern: identifier), infoFlagsOut: &infoFlags
    )
    if status != noErr {
      lock.lock(); pending.removeValue(forKey: identifier); lock.unlock()
      _ = record(status, "encode")
      return .encoderFailure
    }
    if infoFlags.contains(.frameDropped) {
      // A synchronous drop need not produce an output callback. Do not leave
      // its token in the bounded queue or count it twice if VT did call back.
      lock.lock()
      let wasPending = pending.removeValue(forKey: identifier) != nil
      lock.unlock()
      if wasPending { dropped() }
      needsKeyFrame = forceKey
    } else {
      needsKeyFrame = false
    }
    return .ok
  }

  private func receive(sequence: Int, status: OSStatus, flags: VTEncodeInfoFlags, sample: CMSampleBuffer?) {
    lock.lock()
    let timestamp = pending.removeValue(forKey: sequence)
    let callback = self.callback
    lock.unlock()
    guard status == noErr else { _ = record(status, "output"); dropped(); return }
    guard let timestamp, let callback, let sample, !flags.contains(.frameDropped),
      diagnostics.snapshot().ceilingBps > 0 else { dropped(); return }
    do {
      let output = try H264Payload.make(sample)
      diagnostics.update { $0.profile = output.profile }
      // Check delta frames too: a rejected keyframe must not be followed by
      // dependent frames that exceed the negotiated decoder capability.
      guard let profile = output.profile, profile.hasPrefix("640c"),
        let actualLevel = UInt8(profile.suffix(2), radix: 16), actualLevel <= level else {
        _ = record(-50, "ProfileLevel"); dropped(); return
      }
      let dimensions = CMVideoFormatDescriptionGetDimensions(CMSampleBufferGetFormatDescription(sample)!)
      let accepted = callback(EncodedVideoFrame(
        data: output.data, dimensions: Dimensions(width: dimensions.width, height: dimensions.height),
        rtpTimestamp: timestamp, frameType: output.key ? .key : .delta, packetizationMode: .nonInterleaved
      ))
      diagnostics.update { $0.framesEncoded += 1; if !accepted { $0.callbackRejected += 1 } }
    } catch { _ = record(-50, "packetize"); dropped() }
  }

  func releaseEncoder() -> VideoEncoderStatus {
    setCallback(nil)
    closeSession()
    return .ok
  }

  private func closeSession() {
    if let session {
      VTCompressionSessionCompleteFrames(session, untilPresentationTimeStamp: .invalid)
      VTCompressionSessionInvalidate(session)
      self.session = nil
    }
    appliedVbrCeiling = nil
    lock.lock(); pending.removeAll(); lock.unlock()
  }
  private func dropped() { diagnostics.update { $0.framesDropped += 1 } }
  private func record(_ status: OSStatus, _ operation: String) -> Bool {
    // Keep the last error instead of overwriting it with subsequent successes.
    diagnostics.update { if status != noErr || $0.lastStatus == 0 { $0.lastStatus = status; $0.operation = operation } }
    return status == noErr
  }
  deinit { closeSession() }
}

/// Convert VT's AVCC samples to the Annex B format consumed by WebRTC. Key
/// frames carry SPS/PPS so a new receiver can initialize its decoder.
enum H264Payload {
  struct Output { let data: Data; let key: Bool; let profile: String? }
  static func make(_ sample: CMSampleBuffer) throws -> Output {
    guard let format = CMSampleBufferGetFormatDescription(sample), let block = CMSampleBufferGetDataBuffer(sample) else { throw PayloadError.invalid }
    let attachments = CMSampleBufferGetSampleAttachmentsArray(sample, createIfNecessary: false) as? [[CFString: Any]]
    let key = (attachments?.first?[kCMSampleAttachmentKey_NotSync] as? Bool) != true
    var headerLength: Int32 = 0
    var count = 0
    guard CMVideoFormatDescriptionGetH264ParameterSetAtIndex(format, parameterSetIndex: 0, parameterSetPointerOut: nil,
      parameterSetSizeOut: nil, parameterSetCountOut: &count, nalUnitHeaderLengthOut: &headerLength) == noErr else { throw PayloadError.invalid }
    var output = Data()
    var profile: String?
    for index in 0..<count {
      var pointer: UnsafePointer<UInt8>?
      var size = 0
      guard CMVideoFormatDescriptionGetH264ParameterSetAtIndex(format, parameterSetIndex: index, parameterSetPointerOut: &pointer,
        parameterSetSizeOut: &size, parameterSetCountOut: nil, nalUnitHeaderLengthOut: nil) == noErr, let pointer else { throw PayloadError.invalid }
      if key {
        output.append(contentsOf: [0, 0, 0, 1])
        output.append(pointer, count: size)
      }
      if size >= 4, pointer[0] & 0x1f == 7 { profile = (1...3).map { String(format: "%02x", pointer[$0]) }.joined() }
    }
    let length = CMBlockBufferGetDataLength(block)
    guard length > 0 else { throw PayloadError.invalid }
    var avcc = Data(count: length)
    let status = avcc.withUnsafeMutableBytes { CMBlockBufferCopyDataBytes(block, atOffset: 0, dataLength: length, destination: $0.baseAddress!) }
    guard status == noErr else { throw PayloadError.invalid }
    output.append(try annexB(avcc, lengthBytes: Int(headerLength)))
    return Output(data: output, key: key, profile: profile)
  }
  static func annexB(_ input: Data, lengthBytes: Int) throws -> Data {
    guard (1...4).contains(lengthBytes), !input.isEmpty else { throw PayloadError.invalid }
    var offset = 0
    var output = Data()
    while offset < input.count {
      guard input.count - offset >= lengthBytes else { throw PayloadError.invalid }
      let size = input[offset..<(offset + lengthBytes)].reduce(0) { ($0 << 8) | Int($1) }
      offset += lengthBytes
      guard size > 0, size <= input.count - offset else { throw PayloadError.invalid }
      output.append(contentsOf: [0, 0, 0, 1])
      output.append(input[offset..<(offset + size)])
      offset += size
    }
    return output
  }
  enum PayloadError: Error { case invalid }
}
