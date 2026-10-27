// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import CoreGraphics
import Foundation
import ImageIO

enum BenchmarkReport {
  /// Records only test parameters and OS version, never host/user identifiers.
  static func writeEnvironment(_ options: BenchmarkOptions) throws {
    let metadata: [String: String] = [
      "fixtureVersion": "2", "os": ProcessInfo.processInfo.operatingSystemVersionString,
      "width": String(options.width), "height": String(options.height),
      "fps": String(options.fps), "seconds": String(BenchmarkOptions.duration),
      "repeats": String(options.repeats),
      "input": "deterministic BGRA; paced media timestamps; no network",
      "limits": "video payload; average mode uses a 5-second ceiling, VBR uses a VBV maximum; reports compare 5-second output with the configured maximum and 8 Mbps, not instantaneous packet rate",
      "quality": "RGB-weighted full-frame luma PSNR; 4 frames/s sampled; color conversion contributes error",
      "latency": "submission to compression callback, includes encoder queue; excludes capture and network",
    ]
    try VideoBenchmark.writeJSON(metadata, to: options.output.appendingPathComponent("environment.json"))
  }

  static func write(_ results: [BenchmarkResult], options: BenchmarkOptions) throws {
    func number(_ value: Double?) -> String { value.map { String(format: "%.3f", $0) } ?? "" }
    func flag(_ value: Bool?) -> String { value.map { $0 ? "true" : "false" } ?? "" }
    var rows = ["configuration,repeat,hardware,encoded_frames,dropped_frames,average_mbps,peak_1s_mbps,peak_5s_mbps,encode_p95_ms,submit_lateness_max_ms,still_psnr_db,motion_psnr_db,unsupported_properties,exceeds_configured_5s_ceiling,exceeds_8mbps_5s_ceiling,failed"]
    for result in results {
      var columns: [String] = [
        result.configuration.name, String(result.repetition), flag(result.hardware),
        String(result.encodedFrames), String(result.droppedFrames),
      ]
      columns.append(contentsOf: [
        number(result.averageBps.map { $0 / 1_000_000 }),
        number(result.peakOneSecondBps.map { $0 / 1_000_000 }),
        number(result.peakFiveSecondBps.map { $0 / 1_000_000 }),
        number(result.p95EncodeLatencyMs), number(result.maximumSubmitLatenessMs),
        number(result.stillLumaPSNR), number(result.motionLumaPSNR),
      ])
      columns.append(contentsOf: [
        String(result.properties.filter { $0.setStatus != 0 }.count),
        flag(result.exceedsConfiguredFiveSecondCeiling),
        flag(result.exceedsEightMbpsFiveSecondCeiling), String(result.error != nil),
      ])
      rows.append(columns.joined(separator: ","))
    }
    try (rows.joined(separator: "\n") + "\n").write(to: options.output.appendingPathComponent("summary.csv"), atomically: true, encoding: .utf8)
    try comparison(results, options: options)
  }

  /// A contact sheet of matching native-size crops. Full PNGs and movies remain
  /// available beside it; labels include source frame numbers when drops differ.
  private static func comparison(_ results: [BenchmarkResult], options: BenchmarkOptions) throws {
    let runs = results.filter { $0.repetition == 1 && $0.error == nil && $0.snapshotFrame != nil }
    guard let first = runs.first else { return }
    var images: [(String, CGImage)] = []
    func add(_ label: String, _ path: String) {
      if let source = CGImageSourceCreateWithURL(options.output.appendingPathComponent(path) as CFURL, nil),
        let image = CGImageSourceCreateImageAtIndex(source, 0, nil) { images.append((label, image)) }
    }
    add("Reference | frame \(first.snapshotFrame!)", "\(first.configuration.name)-run1/reference.png")
    for run in runs {
      add("\(run.configuration.name) | frame \(run.snapshotFrame!)", "\(run.configuration.name)-run1/decoded.png")
    }
    let width = min(options.width, 640)
    let height = min(options.height, 280)
    let tileHeight = height + 32
    let rows = (images.count + 1) / 2
    guard let context = CGContext(data: nil, width: width * 2, height: tileHeight * rows, bitsPerComponent: 8,
      bytesPerRow: width * 8, space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: BenchmarkScene.bitmapInfo) else {
      throw BenchmarkError(message: "Cannot allocate comparison image")
    }
    context.setFillColor(CGColor(gray: 0.07, alpha: 1))
    context.fill(CGRect(x: 0, y: 0, width: width * 2, height: tileHeight * rows))
    for (index, entry) in images.enumerated() {
      let x = (index % 2) * width
      let y = (rows - 1 - index / 2) * tileHeight
      let rect = CGRect(x: (entry.1.width - width) / 2, y: (entry.1.height - height) / 2, width: width, height: height)
      if let crop = entry.1.cropping(to: rect) { context.draw(crop, in: CGRect(x: x, y: y, width: width, height: height)) }
      BenchmarkScene.text(entry.0, at: CGPoint(x: x + 8, y: y + height + 9), size: 15, context: context)
    }
    guard let image = context.makeImage() else { throw BenchmarkError(message: "Cannot render comparison image") }
    try BenchmarkScene.savePNG(image, to: options.output.appendingPathComponent("comparison.png"))
  }
}
