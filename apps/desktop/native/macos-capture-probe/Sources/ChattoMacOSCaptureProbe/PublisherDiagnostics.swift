// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import Foundation
import LiveKit

/// Only these fields may cross the diagnostic boundary. Never serialize a
/// complete SDK report: it contains addresses, track IDs, and other identifiers.
struct PublisherDiagnostic: Encodable {
  let protocolVersion = 1
  let kind = "diagnostic"
  let phase: String
  let requestedCodec: String
  let simulcast: Bool
  var tunedH264 = false
  var tunedEncoder: TunedEncoderSnapshot?
  let sdkVersion = LiveKitSDK.version
  /// Whether the publish options prefer resolution over frame rate during adaptation.
  var maintainsResolution: Bool?
  /// Congestion-control estimate for the shared publisher transport, including audio.
  var availableOutgoingBps: Double?
  var captureWidth: Int32?
  var captureHeight: Int32?
  var layers: [PublisherLayerDiagnostic] = []

  func write() {
    guard let data = try? JSONEncoder().encode(self) else { return }
    FileHandle.standardOutput.write(data + Data("\n".utf8))
  }
}

struct PublisherLayerDiagnostic: Encodable {
  let codec: String
  let profile: String?
  let encoder: String
  let active: Bool?
  let powerEfficient: Bool?
  let width: UInt?
  let height: UInt?
  let fps: Double?
  let targetBps: Double?
  let actualBps: Double?
  /// Interval measurements use the same WebRTC timestamps as actualBps.
  let intervalMs: Double?
  /// Hypothetical bytes for encoded frames at the encoder target, per second.
  /// This is not a network bandwidth estimate or a guaranteed bitrate.
  let encodedTargetBps: Double?
  let encodedFps: Double?
  let sentFps: Double?
  let captureFps: Double?
  let packetSendDelayMs: Double?
  let meanQP: Double?
  let encodeMsPerFrame: Double?
  let limitation: String
  let packetsLost: Int64?
  let roundTripMs: Double?
  let retransmittedBytes: UInt64?
  let keyFrames: UInt?
  let nackCount: UInt?
  let pliCount: UInt?
}

/// Counter snapshots are internal and keyed by the SDK stream ID. A counter
/// reset or repeated timestamp produces no rate instead of an invalid spike.
struct VideoCounterSample {
  let timestamp: Double // WebRTC statistics timestamps are microseconds.
  let bytes: UInt64?
  let frames: UInt?
  let qp: UInt64?
  let encodeSeconds: Double?
  var targetBytes: UInt64? = nil
  var sentFrames: UInt? = nil
  var sentPackets: UInt64? = nil
  var packetSendSeconds: Double? = nil

  /// Missing or reset counters invalidate only the measurements that use them.
  func rates(since previous: VideoCounterSample?) -> VideoIntervalRates {
    guard let previous, timestamp.isFinite, previous.timestamp.isFinite,
      timestamp > previous.timestamp
    else { return VideoIntervalRates() }
    let seconds = (timestamp - previous.timestamp) / 1_000_000
    let frameCount = Self.delta(frames, previous.frames)
    let packetCount = Self.delta(sentPackets, previous.sentPackets)
    return VideoIntervalRates(
      intervalMs: seconds * 1000,
      bps: Self.delta(bytes, previous.bytes).map { $0 * 8 / seconds },
      qp: Self.perItem(Self.delta(qp, previous.qp), frameCount),
      encodeMs: Self.perItem(Self.delta(encodeSeconds, previous.encodeSeconds), frameCount).map { $0 * 1000 },
      encodedTargetBps: Self.delta(targetBytes, previous.targetBytes).map { $0 * 8 / seconds },
      encodedFps: frameCount.map { $0 / seconds },
      sentFps: Self.delta(sentFrames, previous.sentFrames).map { $0 / seconds },
      packetSendDelayMs: Self.perItem(Self.delta(packetSendSeconds, previous.packetSendSeconds), packetCount).map { $0 * 1000 }
    )
  }

  private static func delta<T: BinaryInteger>(_ current: T?, _ previous: T?) -> Double? {
    guard let current, let previous, current >= previous else { return nil }
    return Double(current - previous)
  }

  private static func delta(_ current: Double?, _ previous: Double?) -> Double? {
    guard let current, let previous, current.isFinite, previous.isFinite,
      current >= previous, previous >= 0 else { return nil }
    return current - previous
  }

  private static func perItem(_ total: Double?, _ count: Double?) -> Double? {
    guard let total, let count, count > 0 else { return nil }
    return total / count
  }
}

/// Derived values remain absent when the SDK does not provide their counters.
struct VideoIntervalRates {
  var intervalMs: Double? = nil
  var bps: Double? = nil
  var qp: Double? = nil
  var encodeMs: Double? = nil
  var encodedTargetBps: Double? = nil
  var encodedFps: Double? = nil
  var sentFps: Double? = nil
  var packetSendDelayMs: Double? = nil
}

enum PublisherDiagnostics {
  /// Runs only for the publisher lifetime. Each sample includes all active
  /// simulcast and backup-codec streams, without exposing their identifiers.
  static func monitor(track: LocalVideoTrack, requestedCodec: String, simulcast: Bool, maintainsResolution: Bool, tunedEncoder: TunedEncoderDiagnostics? = nil) async {
    var previous: [String: VideoCounterSample] = [:]
    let started = ContinuousClock.now
    while !Task.isCancelled {
      // Capture startup adaptation before the ordinary five-second samples.
      let delay = ContinuousClock.now - started < .seconds(20) ? 1 : 5
      do { try await Task.sleep(for: .seconds(delay)) } catch { return }
      var diagnostic = PublisherDiagnostic(phase: "sample", requestedCodec: requestedCodec, simulcast: simulcast)
      diagnostic.maintainsResolution = maintainsResolution
      diagnostic.tunedH264 = tunedEncoder != nil
      diagnostic.tunedEncoder = tunedEncoder?.snapshot()
      diagnostic.captureWidth = track.dimensions?.width
      diagnostic.captureHeight = track.dimensions?.height
      let reports = [track.statistics].compactMap { $0 } + Array(track.simulcastStatistics.values)
      var next: [String: VideoCounterSample] = [:]
      for report in reports {
        // Select the transport's current pair, not an arbitrary nominated or stale
        // candidate. Neither the pair ID nor any address crosses the log boundary.
        if let pairID = report.transportStats?.selectedCandidatePairId,
          let estimate = report.iceCandidatePair.first(where: { $0.id == pairID })?.availableOutgoingBitrate,
          estimate.isFinite, estimate >= 0
        {
          diagnostic.availableOutgoingBps = estimate
        }
        for stream in report.outboundRtpStream {
          let codecStats = report.codec.first { $0.id == stream.codecId }
          let codec = codecStats?.mimeType?.lowercased() ?? ""
          guard codec.hasPrefix("video/") else { continue }
          let sample = VideoCounterSample(
            timestamp: stream.timestamp, bytes: stream.bytesSent,
            frames: stream.framesEncoded, qp: stream.qpSum,
            encodeSeconds: stream.totalEncodeTime,
            targetBytes: stream.totalEncodedBytesTarget,
            sentFrames: stream.framesSent,
            sentPackets: stream.packetsSent,
            packetSendSeconds: stream.totalPacketSendDelay
          )
          let rates = sample.rates(since: previous[stream.id])
          next[stream.id] = sample
          let remote = report.remoteInboundRtpStream.first { $0.localId == stream.id }
          diagnostic.layers.append(PublisherLayerDiagnostic(
            codec: ["video/h264", "video/h265", "video/vp8", "video/vp9", "video/av1"].contains(codec) ? codec : "unknown",
            profile: codecProfile(codecStats?.sdpFmtpLine),
            encoder: encoderCategory(stream.encoderImplementation),
            active: stream.active, powerEfficient: stream.powerEfficientEncoder,
            width: stream.frameWidth, height: stream.frameHeight, fps: stream.framesPerSecond,
            targetBps: stream.targetBitrate, actualBps: rates.bps,
            intervalMs: rates.intervalMs, encodedTargetBps: rates.encodedTargetBps,
            encodedFps: rates.encodedFps, sentFps: rates.sentFps,
            captureFps: report.videoSource.first(where: { $0.id == stream.mediaSourceId })?.framesPerSecond,
            packetSendDelayMs: rates.packetSendDelayMs,
            meanQP: rates.qp,
            encodeMsPerFrame: rates.encodeMs,
            limitation: stream.qualityLimitationReason?.rawValue ?? "unknown",
            packetsLost: remote?.packetsLost, roundTripMs: remote?.roundTripTime.map { $0 * 1000 },
            retransmittedBytes: stream.retransmittedBytesSent,
            keyFrames: stream.keyFramesEncoded, nackCount: stream.nackCount, pliCount: stream.pliCount
          ))
        }
      }
      previous = next
      diagnostic.write()
    }
  }

  /// SDK implementation strings are not copied into reports verbatim.
  static func encoderCategory(_ value: String?) -> String {
    let value = value?.lowercased() ?? ""
    if value == "chattovideotoolboxh264" { return "ChattoVideoToolboxH264" }
    if value.contains("videotoolbox") { return "VideoToolbox" }
    if value.contains("libaom") { return "libaom" }
    if value.contains("libvpx") { return "libvpx" }
    if value.contains("openh264") { return "OpenH264" }
    return "unknown"
  }

  /// Extract only the numeric/hex profile, never the full SDP format line.
  static func codecProfile(_ format: String?) -> String? {
    for parameter in (format ?? "").split(separator: ";") {
      let pair = parameter.split(separator: "=", maxSplits: 1).map { $0.trimmingCharacters(in: .whitespaces) }
      guard pair.count == 2, ["profile-level-id", "profile-id"].contains(pair[0]),
        (1...8).contains(pair[1].count), pair[1].allSatisfy(\.isHexDigit)
      else { continue }
      return pair[1]
    }
    return nil
  }
}
