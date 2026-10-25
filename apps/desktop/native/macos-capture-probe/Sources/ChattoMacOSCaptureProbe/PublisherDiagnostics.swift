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
  let bytes: UInt64
  let frames: UInt
  let qp: UInt64
  let encodeSeconds: Double

  func rates(since previous: VideoCounterSample?) -> (bps: Double?, qp: Double?, encodeMs: Double?) {
    guard let previous, timestamp > previous.timestamp,
      bytes >= previous.bytes, frames >= previous.frames,
      qp >= previous.qp, encodeSeconds >= previous.encodeSeconds
    else { return (nil, nil, nil) }
    let frameCount = Double(frames - previous.frames)
    let bps = Double(bytes - previous.bytes) * 8_000_000 / (timestamp - previous.timestamp)
    return (
      bps,
      frameCount > 0 ? Double(qp - previous.qp) / frameCount : nil,
      frameCount > 0 ? (encodeSeconds - previous.encodeSeconds) * 1000 / frameCount : nil
    )
  }
}

enum PublisherDiagnostics {
  /// Runs only for the publisher lifetime. Each sample includes all active
  /// simulcast and backup-codec streams, without exposing their identifiers.
  static func monitor(track: LocalVideoTrack, requestedCodec: String, simulcast: Bool) async {
    var previous: [String: VideoCounterSample] = [:]
    while !Task.isCancelled {
      do { try await Task.sleep(for: .seconds(5)) } catch { return }
      var diagnostic = PublisherDiagnostic(phase: "sample", requestedCodec: requestedCodec, simulcast: simulcast)
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
            timestamp: stream.timestamp, bytes: stream.bytesSent ?? 0,
            frames: stream.framesEncoded ?? 0, qp: stream.qpSum ?? 0,
            encodeSeconds: stream.totalEncodeTime ?? 0
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
            meanQP: stream.qpSum == nil ? nil : rates.qp,
            encodeMsPerFrame: stream.totalEncodeTime == nil ? nil : rates.encodeMs,
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
