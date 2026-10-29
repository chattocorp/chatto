// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import Foundation
import VideoToolbox

struct BenchmarkError: LocalizedError {
  let message: String
  var errorDescription: String? { message }
}

/// Each variant changes one encoder control relative to its named comparison.
/// Average-rate variants use a five-second ceiling. VBR variants use a VBV
/// maximum instead; neither setting is an instantaneous packet-rate cap.
struct EncoderConfiguration: Codable {
  let name: String
  let comparison: String?
  var targetBps = 6_000_000
  var ceilingBps = 6_000_000
  var speedPriority = true
  var highProfile = false
  var lowLatency = false
  var maximumQP = 39
  var hevc = false
  var constrainedHigh = false
  /// A non-nil duration selects macOS 26 VBR and replaces AverageBitRate/DataRateLimits.
  var vbrBufferSeconds: Double?

  var codec: CMVideoCodecType { hevc ? kCMVideoCodecType_HEVC : kCMVideoCodecType_H264 }
  var profile: CFString {
    if hevc { return kVTProfileLevel_HEVC_Main_AutoLevel }
    if constrainedHigh { return kVTProfileLevel_H264_ConstrainedHigh_AutoLevel }
    // WebRTC 150.7871.03 uses Baseline AutoLevel for screen sharing, even
    // when SDP advertises level 5.2. Its High-family mapping uses High 5.2.
    return highProfile ? kVTProfileLevel_H264_High_5_2 : kVTProfileLevel_H264_Baseline_AutoLevel
  }

  static let matrix: [Self] = [
    Self(name: "baseline-6", comparison: nil),
    Self(name: "quality-6", comparison: "baseline-6", speedPriority: false),
    Self(name: "headroom-6", comparison: "baseline-6", ceilingBps: 8_000_000),
    Self(name: "high-6", comparison: "baseline-6", highProfile: true),
    Self(name: "high-headroom-6", comparison: "high-6", ceilingBps: 8_000_000, highProfile: true),
    Self(name: "high-low-latency-6", comparison: "high-6", highProfile: true, lowLatency: true),
    Self(name: "qp32-6", comparison: "baseline-6", maximumQP: 32),
    Self(name: "baseline-8", comparison: "baseline-6", targetBps: 8_000_000, ceilingBps: 8_000_000),
    Self(name: "hevc-6", comparison: "baseline-6", hevc: true),
    Self(name: "tuned-6", comparison: nil, ceilingBps: 8_000_000, constrainedHigh: true),
    Self(name: "tuned-vbr-100ms-6", comparison: "tuned-6", ceilingBps: 8_000_000, constrainedHigh: true, vbrBufferSeconds: 0.1),
    Self(name: "tuned-vbr-250ms-6", comparison: "tuned-6", ceilingBps: 8_000_000, constrainedHigh: true, vbrBufferSeconds: 0.25),
  ]
}

struct BenchmarkOptions {
  let output: URL
  var width = 1920
  var height = 1122
  var fps = 60
  var repeats = 2
  var configurations = EncoderConfiguration.matrix
  static let duration = 6

  static let usage = """
    Usage: chatto-video-benchmark --output <new-directory>
      [--width 1920] [--height 1122] [--fps 60] [--repeats 2]
      [--only baseline-6,quality-6,...]
    Runs a six-second deterministic scene: still, slow pan, fast pan.
    Repeats alternate forward/reverse configuration order. No screen capture or network.
    The output directory must not exist. Hardware video encoding is required.
    """

  static func parse(_ arguments: [String]) throws -> Self {
    guard arguments.count % 2 == 0 else { throw BenchmarkError(message: usage) }
    var values: [String: String] = [:]
    for index in stride(from: 0, to: arguments.count, by: 2) {
      let key = arguments[index]
      guard ["--output", "--width", "--height", "--fps", "--repeats", "--only"].contains(key),
        values[key] == nil else { throw BenchmarkError(message: "Unknown or repeated option: \(key)") }
      values[key] = arguments[index + 1]
    }
    guard let path = values["--output"], !path.isEmpty else { throw BenchmarkError(message: usage) }
    var result = Self(output: URL(fileURLWithPath: path, isDirectory: true))
    func integer(_ key: String, _ fallback: Int, _ range: ClosedRange<Int>) throws -> Int {
      guard let value = values[key] else { return fallback }
      guard let number = Int(value), range.contains(number) else {
        throw BenchmarkError(message: "Invalid value for \(key)")
      }
      return number
    }
    result.width = try integer("--width", result.width, 64...3840)
    result.height = try integer("--height", result.height, 64...2160)
    result.fps = try integer("--fps", result.fps, 1...60)
    result.repeats = try integer("--repeats", result.repeats, 1...3)
    guard result.width % 2 == 0, result.height % 2 == 0 else {
      throw BenchmarkError(message: "Dimensions must be even.")
    }
    if let only = values["--only"] {
      let names = only.split(separator: ",").map(String.init)
      guard !names.isEmpty, Set(names).count == names.count,
        names.allSatisfy({ name in EncoderConfiguration.matrix.contains { $0.name == name } }) else {
        throw BenchmarkError(message: "Unknown or repeated configuration in --only.")
      }
      result.configurations = EncoderConfiguration.matrix.filter { names.contains($0.name) }
    }
    return result
  }
}

/// Per-frame measurements use source media time, including gaps from frame drops.
struct EncodedFrameMeasurement: Codable {
  let frame: Int
  let time: Double
  let bytes: Int
  let latencyMs: Double
}

enum BenchmarkMath {
  /// Maximum bytes in a half-open media-time window [t, t + seconds).
  static func peakBps(_ frames: [EncodedFrameMeasurement], seconds: Double) -> Double {
    let ordered = frames.sorted { $0.time < $1.time }
    var end = 0
    var bytes = 0
    var peak = 0
    for start in ordered.indices {
      while end < ordered.count && ordered[end].time < ordered[start].time + seconds {
        bytes += ordered[end].bytes
        end += 1
      }
      peak = max(peak, bytes)
      bytes -= ordered[start].bytes
    }
    return Double(peak) * 8 / seconds
  }

  static func percentile(_ values: [Double], _ fraction: Double) -> Double? {
    guard !values.isEmpty else { return nil }
    let ordered = values.sorted()
    return ordered[min(ordered.count - 1, Int(Double(ordered.count - 1) * fraction))]
  }
}
