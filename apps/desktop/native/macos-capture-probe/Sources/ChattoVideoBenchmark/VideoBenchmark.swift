// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import AVFoundation
import CoreMedia
import CoreVideo
import Foundation
import VideoToolbox

struct PropertyMeasurement: Codable {
  let property: String
  let requested: String
  let setStatus: OSStatus
  let readStatus: OSStatus
  let actual: String?
}

struct QualityMeasurement: Codable {
  let frame: Int
  let phase: String
  let lumaMSE: Double
  var lumaPSNR: Double? { lumaMSE > 0 ? 10 * log10(255 * 255 / lumaMSE) : nil }
}

struct BenchmarkResult: Codable {
  let configuration: EncoderConfiguration
  let repetition: Int
  var error: String?
  var hardware: Bool?
  var properties: [PropertyMeasurement] = []
  var encodedFrames = 0
  var droppedFrames = 0
  var decodedFrames = 0
  var averageBps: Double?
  var peakOneSecondBps: Double?
  var peakFiveSecondBps: Double?
  var exceedsConfiguredFiveSecondCeiling: Bool?
  var exceedsEightMbpsFiveSecondCeiling: Bool?
  var medianEncodeLatencyMs: Double?
  var p95EncodeLatencyMs: Double?
  var maximumSubmitLatenessMs: Double?
  var quality: [QualityMeasurement] = []
  var motionLumaPSNR: Double?
  var stillLumaPSNR: Double?
  var snapshotFrame: Int?
}

/// VideoToolbox invokes this sink from encoder threads. The lock owns writer
/// setup, append, errors, and measurements. Encoding never holds this lock.
private final class EncodedMovieSink: @unchecked Sendable {
  private let lock = NSLock()
  let writer: AVAssetWriter
  private var input: AVAssetWriterInput?
  private var submitted: [Int: Double] = [:]
  private(set) var frames: [EncodedFrameMeasurement] = []
  private var failure: String?

  init(url: URL) throws { writer = try AVAssetWriter(outputURL: url, fileType: .mov) }

  func submitted(_ index: Int, at time: Double) {
    lock.lock()
    defer { lock.unlock() }
    submitted[index] = time
  }

  func receive(index: Int, status: OSStatus, sample: CMSampleBuffer?) {
    let completed = ProcessInfo.processInfo.systemUptime
    lock.lock()
    defer { lock.unlock() }
    guard failure == nil else { return }
    guard status == noErr else { failure = "Encoder callback failed: \(status)"; return }
    // A nil sample is a permitted encoder drop. Final counts include the gap.
    guard let sample, CMSampleBufferDataIsReady(sample) else { return }
    guard let submittedAt = submitted[index] else { failure = "Missing source timestamp"; return }
    if input == nil {
      let newInput = AVAssetWriterInput(mediaType: .video, outputSettings: nil, sourceFormatHint: CMSampleBufferGetFormatDescription(sample))
      newInput.expectsMediaDataInRealTime = false
      guard writer.canAdd(newInput) else { failure = "Cannot add compressed movie input"; return }
      writer.add(newInput)
      guard writer.startWriting() else { failure = "Cannot start movie writer"; return }
      writer.startSession(atSourceTime: .zero)
      input = newInput
    }
    guard let input, input.isReadyForMoreMediaData, input.append(sample) else {
      failure = "Movie writer could not accept output; run is invalid"
      return
    }
    frames.append(EncodedFrameMeasurement(
      frame: index, time: CMTimeGetSeconds(CMSampleBufferGetPresentationTimeStamp(sample)),
      bytes: CMSampleBufferGetTotalSampleSize(sample), latencyMs: (completed - submittedAt) * 1000
    ))
  }

  /// Call after CompleteFrames, when no more encode callbacks can arrive.
  func finish() async throws {
    if let failure { writer.cancelWriting(); throw BenchmarkError(message: failure) }
    guard let input, !frames.isEmpty else { throw BenchmarkError(message: "Encoder produced no frames") }
    input.markAsFinished()
    await writer.finishWriting()
    guard writer.status == .completed else { throw BenchmarkError(message: "Movie writer failed") }
  }
}

enum VideoBenchmark {
  static func encode(
    _ configuration: EncoderConfiguration, repetition: Int, options: BenchmarkOptions,
    scene: BenchmarkScene, directory: URL
  ) async -> BenchmarkResult {
    var result = BenchmarkResult(configuration: configuration, repetition: repetition)
    do {
      let sink = try EncodedMovieSink(url: directory.appendingPathComponent("encoded.mov"))
      defer { if sink.writer.status == .writing { sink.writer.cancelWriting() } }
      var session: VTCompressionSession?
      var specification: [CFString: Any] = [kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder: true]
      if configuration.lowLatency { specification[kVTVideoEncoderSpecification_EnableLowLatencyRateControl] = true }
      try check(VTCompressionSessionCreate(
        allocator: kCFAllocatorDefault, width: Int32(options.width), height: Int32(options.height),
        codecType: configuration.codec, encoderSpecification: specification as CFDictionary,
        imageBufferAttributes: [kCVPixelBufferPixelFormatTypeKey: kCVPixelFormatType_32BGRA] as CFDictionary,
        compressedDataAllocator: nil,
        outputCallback: { refcon, source, status, _, sample in
          guard let refcon, let source else { return }
          Unmanaged<EncodedMovieSink>.fromOpaque(refcon).takeUnretainedValue().receive(
            index: Int(bitPattern: source) - 1, status: status, sample: sample
          )
        }, refcon: Unmanaged.passUnretained(sink).toOpaque(), compressionSessionOut: &session
      ), "Create hardware encoder")
      guard let session else { throw BenchmarkError(message: "No compression session") }
      defer { VTCompressionSessionInvalidate(session) }

      // Read back each setting. An unsupported optional control remains visible
      // in the report instead of silently looking like a successful experiment.
      func set(_ key: CFString, _ value: CFTypeRef, required: Bool = true) throws {
        let status = VTSessionSetProperty(session, key: key, value: value)
        var actual: Unmanaged<CFTypeRef>?
        let readStatus = VTSessionCopyProperty(session, key: key, allocator: nil, valueOut: &actual)
        result.properties.append(PropertyMeasurement(
          property: key as String, requested: String(describing: value), setStatus: status,
          readStatus: readStatus, actual: actual.map { String(describing: $0.takeRetainedValue()) }
        ))
        if required { try check(status, "Set \(key)") }
      }
      try set(kVTCompressionPropertyKey_RealTime, kCFBooleanTrue)
      try set(kVTCompressionPropertyKey_ProfileLevel, configuration.profile)
      try set(kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse)
      try set(kVTCompressionPropertyKey_ExpectedFrameRate, options.fps as CFNumber)
      try set(kVTCompressionPropertyKey_MaxKeyFrameInterval, 7200 as CFNumber)
      try set(kVTCompressionPropertyKey_MaxKeyFrameIntervalDuration, 240 as CFNumber)
      try set(kVTCompressionPropertyKey_PrioritizeEncodingSpeedOverQuality, configuration.speedPriority ? kCFBooleanTrue : kCFBooleanFalse, required: false)
      try set(kVTCompressionPropertyKey_MaxAllowedFrameQP, configuration.maximumQP as CFNumber, required: false)
      if let bufferSeconds = configuration.vbrBufferSeconds {
        guard #available(macOS 26, *) else { throw BenchmarkError(message: "VBR requires macOS 26 or later") }
        // VBR is incompatible with AverageBitRate and DataRateLimits. A small
        // VBV and zero initial delay test suitability for interactive sharing.
        // Documented names avoid requiring the macOS 26 SDK at build time.
        // OS availability and each hardware property's support are checked.
        try set("VariableBitRate" as CFString, configuration.targetBps as CFNumber)
        try set("VBVMaxBitRate" as CFString, configuration.ceilingBps as CFNumber)
        try set("VBVBufferDuration" as CFString, bufferSeconds as CFNumber)
        try set("VBVInitialDelayPercentage" as CFString, 0 as CFNumber)
      } else {
        try set(kVTCompressionPropertyKey_AverageBitRate, configuration.targetBps as CFNumber)
        // Match the SDK's 10x one-second burst allowance and five-second ceiling.
        try set(kVTCompressionPropertyKey_DataRateLimits, [configuration.targetBps * 10 / 8, 1, configuration.ceilingBps * 5 / 8, 5] as CFArray)
      }
      try check(VTCompressionSessionPrepareToEncodeFrames(session), "Prepare encoder")
      var hardware: Unmanaged<CFTypeRef>?
      try check(VTSessionCopyProperty(session, key: kVTCompressionPropertyKey_UsingHardwareAcceleratedVideoEncoder, allocator: nil, valueOut: &hardware), "Read hardware encoder status")
      result.hardware = (hardware?.takeRetainedValue() as? NSNumber)?.boolValue
      guard result.hardware == true else { throw BenchmarkError(message: "Hardware encoding was not confirmed") }

      let started = ProcessInfo.processInfo.systemUptime
      var maximumLateness = 0.0
      for index in 0..<(options.fps * BenchmarkOptions.duration) {
        let buffer = try autoreleasepool { try scene.frame(index) }
        let due = started + Double(index) / Double(options.fps)
        let wait = due - ProcessInfo.processInfo.systemUptime
        if wait > 0 { try await Task.sleep(for: .seconds(wait)) }
        let submitted = ProcessInfo.processInfo.systemUptime
        maximumLateness = max(maximumLateness, submitted - due)
        sink.submitted(index, at: submitted)
        try check(VTCompressionSessionEncodeFrame(
          session, imageBuffer: buffer,
          presentationTimeStamp: CMTime(value: Int64(index), timescale: Int32(options.fps)),
          duration: CMTime(value: 1, timescale: Int32(options.fps)), frameProperties: nil,
          sourceFrameRefcon: UnsafeMutableRawPointer(bitPattern: index + 1), infoFlagsOut: nil
        ), "Encode frame")
      }
      try check(VTCompressionSessionCompleteFrames(session, untilPresentationTimeStamp: .invalid), "Drain encoder")
      try await sink.finish()
      result.encodedFrames = sink.frames.count
      result.droppedFrames = options.fps * BenchmarkOptions.duration - sink.frames.count
      result.averageBps = Double(sink.frames.reduce(0) { $0 + $1.bytes }) * 8 / Double(BenchmarkOptions.duration)
      result.peakOneSecondBps = BenchmarkMath.peakBps(sink.frames, seconds: 1)
      result.peakFiveSecondBps = BenchmarkMath.peakBps(sink.frames, seconds: 5)
      result.exceedsConfiguredFiveSecondCeiling = result.peakFiveSecondBps.map { $0 > Double(configuration.ceilingBps) }
      result.exceedsEightMbpsFiveSecondCeiling = result.peakFiveSecondBps.map { $0 > 8_000_000 }
      result.medianEncodeLatencyMs = BenchmarkMath.percentile(sink.frames.map(\.latencyMs), 0.5)
      result.p95EncodeLatencyMs = BenchmarkMath.percentile(sink.frames.map(\.latencyMs), 0.95)
      result.maximumSubmitLatenessMs = maximumLateness * 1000
      try writeJSON(sink.frames, to: directory.appendingPathComponent("frames.json"))
      try await measureQuality(scene: scene, directory: directory, result: &result)
    } catch { result.error = error.localizedDescription }
    return result
  }

  private static func measureQuality(scene: BenchmarkScene, directory: URL, result: inout BenchmarkResult) async throws {
    let asset = AVURLAsset(url: directory.appendingPathComponent("encoded.mov"))
    guard let track = try await asset.loadTracks(withMediaType: .video).first else {
      throw BenchmarkError(message: "No encoded movie track")
    }
    let reader = try AVAssetReader(asset: asset)
    let output = AVAssetReaderTrackOutput(track: track, outputSettings: [kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_32BGRA])
    reader.add(output)
    guard reader.startReading() else { throw BenchmarkError(message: "Cannot decode movie") }
    while let sample = output.copyNextSampleBuffer() {
      try autoreleasepool {
        guard let decoded = CMSampleBufferGetImageBuffer(sample) else { throw BenchmarkError(message: "Decoded frame has no pixels") }
        let index = Int((CMTimeGetSeconds(CMSampleBufferGetPresentationTimeStamp(sample)) * Double(scene.fps)).rounded())
        result.decodedFrames += 1
        guard index % max(1, scene.fps / 4) == 0 || (index >= scene.fps * 5 && result.snapshotFrame == nil) else { return }
        let reference = try scene.frame(index)
        let mse = try BenchmarkScene.lumaMSE(reference, decoded)
        let phase = index < scene.fps * 2 ? "still" : (index < scene.fps * 4 ? "slow" : "fast")
        result.quality.append(QualityMeasurement(frame: index, phase: phase, lumaMSE: mse))
        if index >= scene.fps * 5 && result.snapshotFrame == nil {
          result.snapshotFrame = index
          try BenchmarkScene.savePNG(BenchmarkScene.image(decoded), to: directory.appendingPathComponent("decoded.png"))
          try BenchmarkScene.savePNG(BenchmarkScene.image(reference), to: directory.appendingPathComponent("reference.png"))
        }
      }
    }
    guard reader.status == .completed, result.decodedFrames == result.encodedFrames else {
      throw BenchmarkError(message: "Decoded frame count differs from encoder output")
    }
    func psnr(_ measurements: [QualityMeasurement]) -> Double? {
      guard !measurements.isEmpty else { return nil }
      let mse = measurements.reduce(0) { $0 + $1.lumaMSE } / Double(measurements.count)
      return mse > 0 ? 10 * log10(255 * 255 / mse) : nil
    }
    result.stillLumaPSNR = psnr(result.quality.filter { $0.phase == "still" })
    result.motionLumaPSNR = psnr(result.quality.filter { $0.phase != "still" })
  }

  static func writeJSON<T: Encodable>(_ value: T, to url: URL) throws {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
    try encoder.encode(value).write(to: url, options: .atomic)
  }

  private static func check(_ status: OSStatus, _ operation: String) throws {
    guard status == noErr else { throw BenchmarkError(message: "\(operation) failed: \(status)") }
  }
}

@main
enum BenchmarkMain {
  static func main() async {
    do {
      if CommandLine.arguments.dropFirst().contains("--help") {
        print(BenchmarkOptions.usage)
        return
      }
      let options = try BenchmarkOptions.parse(Array(CommandLine.arguments.dropFirst()))
      guard !FileManager.default.fileExists(atPath: options.output.path) else {
        throw BenchmarkError(message: "Output directory already exists; choose a new path.")
      }
      try FileManager.default.createDirectory(at: options.output, withIntermediateDirectories: true)
      try BenchmarkReport.writeEnvironment(options)
      let scene = try BenchmarkScene(width: options.width, height: options.height, fps: options.fps)
      var results: [BenchmarkResult] = []
      print("Synthetic fixture: \(options.width)x\(options.height) at \(options.fps) FPS, 6 seconds per run")
      for repetition in 1...options.repeats {
        let configurations = repetition % 2 == 0 ? Array(options.configurations.reversed()) : options.configurations
        for configuration in configurations {
          let name = "\(configuration.name)-run\(repetition)"
          let directory = options.output.appendingPathComponent(name, isDirectory: true)
          try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: false)
          print("Encoding \(name)...")
          let result = await VideoBenchmark.encode(configuration, repetition: repetition, options: options, scene: scene, directory: directory)
          results.append(result)
          try VideoBenchmark.writeJSON(result, to: directory.appendingPathComponent("result.json"))
          try VideoBenchmark.writeJSON(results, to: options.output.appendingPathComponent("results.json"))
          try BenchmarkReport.write(results, options: options)
          if let error = result.error { print("  FAILED: \(error)") }
          else { print("  \(result.encodedFrames) frames, \(Int(result.averageBps ?? 0)) bps, motion PSNR \(String(format: "%.2f", result.motionLumaPSNR ?? 0)) dB") }
        }
      }
      if results.contains(where: { $0.error != nil }) { throw BenchmarkError(message: "Some benchmark runs failed; inspect results.json.") }
    } catch {
      FileHandle.standardError.write(Data("\(error.localizedDescription)\n".utf8))
      exit(1)
    }
  }
}
