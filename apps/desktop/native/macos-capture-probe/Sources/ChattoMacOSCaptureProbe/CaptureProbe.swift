// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import AVFoundation
import AppKit
import AudioToolbox
import CoreMedia
import CoreVideo
import Darwin
import Foundation
@preconcurrency import ScreenCaptureKit
import VideoToolbox

private let defaultDuration = 15.0
private let defaultFrameRate = 60
private let defaultMaximumWidth = 1920
private let defaultMaximumHeight = 1080

private enum ProbeError: LocalizedError {
  case invalidArguments(String)
  case sourceNotFound(String)

  var errorDescription: String? {
    switch self {
    case .invalidArguments(let message), .sourceNotFound(let message):
      message
    }
  }
}

struct CaptureOptions {
  enum Source {
    case display(CGDirectDisplayID)
    case window(CGWindowID)
    case application(bundleIdentifier: String, displayID: CGDirectDisplayID?)
  }

  let source: Source
  let duration: TimeInterval
  let frameRate: Int
  let maximumWidth: Int
  let maximumHeight: Int
  let outputURL: URL?
}

private enum Command {
  case help
  case list
  case listJSON
  case capture(CaptureOptions)
  case publish(CaptureOptions)

  static func parse(_ arguments: ArraySlice<String>) throws -> Command {
    guard let command = arguments.first else {
      throw ProbeError.invalidArguments(usage)
    }

    switch command {
    case "list":
      guard arguments.count == 1 else {
        throw ProbeError.invalidArguments("The list command takes no options.\n\n\(usage)")
      }
      return .list
    case "list-json":
      guard arguments.count == 1 else {
        throw ProbeError.invalidArguments("The list-json command takes no options.\n\n\(usage)")
      }
      return .listJSON
    case "capture":
      return .capture(try parseCapture(arguments.dropFirst()))
    case "publish":
      let options = try parseCapture(arguments.dropFirst())
      guard options.outputURL == nil else {
        throw ProbeError.invalidArguments("The publish command does not accept --output.")
      }
      guard case .window = options.source else {
        throw ProbeError.invalidArguments("The publish command currently requires --window.")
      }
      return .publish(options)
    case "help", "--help", "-h":
      return .help
    default:
      throw ProbeError.invalidArguments("Unknown command: \(command)\n\n\(usage)")
    }
  }

  private static func parseCapture(_ arguments: ArraySlice<String>) throws -> CaptureOptions {
    var displayID: CGDirectDisplayID?
    var windowID: CGWindowID?
    var applicationBundleIdentifier: String?
    var applicationDisplayID: CGDirectDisplayID?
    var duration = defaultDuration
    var frameRate = defaultFrameRate
    var maximumWidth = defaultMaximumWidth
    var maximumHeight = defaultMaximumHeight
    var outputURL: URL?

    var index = arguments.startIndex
    while index < arguments.endIndex {
      let option = arguments[index]
      let valueIndex = arguments.index(after: index)
      guard valueIndex < arguments.endIndex else {
        throw ProbeError.invalidArguments("Missing value for \(option).")
      }
      let value = arguments[valueIndex]

      switch option {
      case "--display":
        displayID = try parseIdentifier(value, option: option)
      case "--window":
        windowID = try parseIdentifier(value, option: option)
      case "--application":
        applicationBundleIdentifier = value
      case "--on-display":
        applicationDisplayID = try parseIdentifier(value, option: option)
      case "--duration":
        guard let parsed = TimeInterval(value), parsed > 0 else {
          throw ProbeError.invalidArguments("\(option) must be greater than zero.")
        }
        duration = parsed
      case "--fps":
        guard let parsed = Int(value), parsed > 0, parsed <= 240 else {
          throw ProbeError.invalidArguments("\(option) must be between 1 and 240.")
        }
        frameRate = parsed
      case "--max-width":
        maximumWidth = try parseDimension(value, option: option)
      case "--max-height":
        maximumHeight = try parseDimension(value, option: option)
      case "--output":
        outputURL =
          URL(
            fileURLWithPath: value,
            relativeTo: URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
          ).standardizedFileURL
      default:
        throw ProbeError.invalidArguments("Unknown option: \(option)")
      }

      index = arguments.index(after: valueIndex)
    }

    let selectedSourceCount = [
      displayID != nil, windowID != nil, applicationBundleIdentifier != nil,
    ]
    .filter { $0 }
    .count
    guard selectedSourceCount == 1 else {
      throw ProbeError.invalidArguments(
        "Choose exactly one source with --display, --window, or --application."
      )
    }
    guard applicationBundleIdentifier != nil || applicationDisplayID == nil else {
      throw ProbeError.invalidArguments("--on-display can only be used with --application.")
    }

    let source: CaptureOptions.Source
    if let displayID {
      source = .display(displayID)
    } else if let windowID {
      source = .window(windowID)
    } else {
      source = .application(
        bundleIdentifier: applicationBundleIdentifier!,
        displayID: applicationDisplayID
      )
    }

    return CaptureOptions(
      source: source,
      duration: duration,
      frameRate: frameRate,
      maximumWidth: maximumWidth,
      maximumHeight: maximumHeight,
      outputURL: outputURL
    )
  }

  private static func parseIdentifier<T: FixedWidthInteger>(
    _ value: String,
    option: String
  ) throws -> T {
    guard let identifier = T(value) else {
      throw ProbeError.invalidArguments("\(option) requires a numeric identifier.")
    }
    return identifier
  }

  private static func parseDimension(_ value: String, option: String) throws -> Int {
    guard let dimension = Int(value), dimension > 0, dimension <= 16384 else {
      throw ProbeError.invalidArguments("\(option) must be between 1 and 16384.")
    }
    return dimension
  }
}

private let usage = """
  Usage:
    chatto-macos-capture-probe list
    chatto-macos-capture-probe list-json
    chatto-macos-capture-probe capture --display <id> [capture options]
    chatto-macos-capture-probe capture --window <id> [capture options]
    chatto-macos-capture-probe capture --application <bundle-id> [--on-display <id>] [capture options]
    chatto-macos-capture-probe publish --window <id> [capture options]

  Capture options:
    --duration <seconds>   Capture duration (default: 15)
    --fps <frames>         Requested frame rate (default: 60, maximum: 240)
    --max-width <pixels>   Maximum output width (default: 1920)
    --max-height <pixels>  Maximum output height (default: 1080)
    --output <file.mov>    Record captured video and audio to a QuickTime movie
  """

private struct PreparedCapture {
  let description: String
  let filter: SCContentFilter
  let sourceWidth: Int
  let sourceHeight: Int
}

private struct SourceList: Encodable {
  let protocolVersion = 1
  let windows: [WindowSource]
}

private struct WindowSource: Encodable {
  let windowID: CGWindowID
  let applicationName: String
  let title: String
  let bundleIdentifier: String
  let width: Int
  let height: Int
}

private struct MetricsSnapshot {
  let totalVideoBuffers: Int
  let completeVideoFrames: Int
  let nonCompleteVideoBuffers: Int
  let inferredVideoGaps: Int
  let firstVideoPTS: Double?
  let lastVideoPTS: Double?
  let videoWidth: Int
  let videoHeight: Int
  let contentSamples: Int
  let changedContentSamples: Int
  let minimumLuma: UInt8
  let maximumLuma: UInt8
  let averageLuma: Double
  let audioBuffers: Int
  let audioFrames: Int
  let firstAudioPTS: Double?
  let lastAudioPTS: Double?
  let audioSampleRate: Double
  let audioChannels: UInt32
  let audioPeak: Float
  let stopError: String?
  let recordingStarted: Bool
  let recordingFinished: Bool
  let recordingError: String?

  var videoFramesPerSecond: Double {
    guard
      completeVideoFrames > 1,
      let firstVideoPTS,
      let lastVideoPTS,
      lastVideoPTS > firstVideoPTS
    else { return 0 }
    return Double(completeVideoFrames - 1) / (lastVideoPTS - firstVideoPTS)
  }

  var audioDuration: Double {
    guard let firstAudioPTS, let lastAudioPTS else { return 0 }
    return max(0, lastAudioPTS - firstAudioPTS)
  }
}

private final class SampleCollector: NSObject, SCStreamOutput, SCStreamDelegate,
  SCRecordingOutputDelegate, @unchecked Sendable
{
  private let expectedFrameInterval: Double
  private let lock = NSLock()

  private var totalVideoBuffers = 0
  private var completeVideoFrames = 0
  private var nonCompleteVideoBuffers = 0
  private var inferredVideoGaps = 0
  private var lastVideoBufferPTS: Double?
  private var firstVideoPTS: Double?
  private var lastVideoPTS: Double?
  private var videoWidth = 0
  private var videoHeight = 0
  private var contentSamples = 0
  private var changedContentSamples = 0
  private var lastContentSignature: UInt64?
  private var minimumLuma = UInt8.max
  private var maximumLuma = UInt8.min
  private var lumaTotal = 0.0

  private var audioBuffers = 0
  private var audioFrames = 0
  private var firstAudioPTS: Double?
  private var lastAudioPTS: Double?
  private var audioSampleRate = 0.0
  private var audioChannels: UInt32 = 0
  private var audioPeak: Float = 0
  private var stopError: String?
  private var recordingStarted = false
  private var recordingFinished = false
  private var recordingError: String?

  init(expectedFrameRate: Int) {
    expectedFrameInterval = 1.0 / Double(expectedFrameRate)
  }

  func stream(
    _ stream: SCStream,
    didOutputSampleBuffer sampleBuffer: CMSampleBuffer,
    of outputType: SCStreamOutputType
  ) {
    guard sampleBuffer.isValid else { return }

    switch outputType {
    case .screen:
      recordVideo(sampleBuffer)
    case .audio:
      recordAudio(sampleBuffer)
    case .microphone:
      break
    @unknown default:
      break
    }
  }

  func stream(_ stream: SCStream, didStopWithError error: any Error) {
    lock.withLock {
      stopError = error.localizedDescription
    }
  }

  func recordingOutputDidStartRecording(_ recordingOutput: SCRecordingOutput) {
    lock.withLock {
      recordingStarted = true
    }
  }

  func recordingOutputDidFinishRecording(_ recordingOutput: SCRecordingOutput) {
    lock.withLock {
      recordingFinished = true
    }
  }

  func recordingOutput(_ recordingOutput: SCRecordingOutput, didFailWithError error: any Error) {
    lock.withLock {
      recordingError = error.localizedDescription
    }
  }

  func snapshot() -> MetricsSnapshot {
    lock.withLock {
      MetricsSnapshot(
        totalVideoBuffers: totalVideoBuffers,
        completeVideoFrames: completeVideoFrames,
        nonCompleteVideoBuffers: nonCompleteVideoBuffers,
        inferredVideoGaps: inferredVideoGaps,
        firstVideoPTS: firstVideoPTS,
        lastVideoPTS: lastVideoPTS,
        videoWidth: videoWidth,
        videoHeight: videoHeight,
        contentSamples: contentSamples,
        changedContentSamples: changedContentSamples,
        minimumLuma: contentSamples == 0 ? 0 : minimumLuma,
        maximumLuma: maximumLuma,
        averageLuma: contentSamples == 0 ? 0 : lumaTotal / Double(contentSamples),
        audioBuffers: audioBuffers,
        audioFrames: audioFrames,
        firstAudioPTS: firstAudioPTS,
        lastAudioPTS: lastAudioPTS,
        audioSampleRate: audioSampleRate,
        audioChannels: audioChannels,
        audioPeak: audioPeak,
        stopError: stopError,
        recordingStarted: recordingStarted,
        recordingFinished: recordingFinished,
        recordingError: recordingError
      )
    }
  }

  private func recordVideo(_ sampleBuffer: CMSampleBuffer) {
    let presentationTime = sampleBuffer.presentationTimeStamp.seconds
    let status = frameStatus(for: sampleBuffer)
    let dimensions = sampleBuffer.imageBuffer.map {
      (CVPixelBufferGetWidth($0), CVPixelBufferGetHeight($0))
    }
    let content = sampleBuffer.imageBuffer.flatMap(sampleContent)

    lock.withLock {
      totalVideoBuffers += 1

      if presentationTime.isFinite, let previousPTS = lastVideoBufferPTS {
        let interval = presentationTime - previousPTS
        if interval > expectedFrameInterval * 1.75 {
          inferredVideoGaps += max(1, Int((interval / expectedFrameInterval).rounded()) - 1)
        }
      }
      if presentationTime.isFinite {
        lastVideoBufferPTS = presentationTime
      }

      guard status == .complete else {
        nonCompleteVideoBuffers += 1
        return
      }

      completeVideoFrames += 1
      firstVideoPTS = firstVideoPTS ?? presentationTime
      lastVideoPTS = presentationTime
      if let dimensions {
        videoWidth = dimensions.0
        videoHeight = dimensions.1
      }
      if let content {
        contentSamples += 1
        if let lastContentSignature, lastContentSignature != content.signature {
          changedContentSamples += 1
        }
        lastContentSignature = content.signature
        minimumLuma = min(minimumLuma, content.minimumLuma)
        maximumLuma = max(maximumLuma, content.maximumLuma)
        lumaTotal += content.averageLuma
      }
    }
  }

  private func sampleContent(
    _ pixelBuffer: CVPixelBuffer
  ) -> (signature: UInt64, minimumLuma: UInt8, maximumLuma: UInt8, averageLuma: Double)? {
    guard CVPixelBufferGetPixelFormatType(pixelBuffer) == kCVPixelFormatType_32BGRA else {
      return nil
    }
    guard CVPixelBufferLockBaseAddress(pixelBuffer, .readOnly) == kCVReturnSuccess else {
      return nil
    }
    defer { CVPixelBufferUnlockBaseAddress(pixelBuffer, .readOnly) }
    guard let baseAddress = CVPixelBufferGetBaseAddress(pixelBuffer) else { return nil }

    let width = CVPixelBufferGetWidth(pixelBuffer)
    let height = CVPixelBufferGetHeight(pixelBuffer)
    let bytesPerRow = CVPixelBufferGetBytesPerRow(pixelBuffer)
    guard width > 0, height > 0 else { return nil }

    let horizontalSamples = min(32, width)
    let verticalSamples = min(18, height)
    let bytes = baseAddress.assumingMemoryBound(to: UInt8.self)
    var signature: UInt64 = 14_695_981_039_346_656_037
    var minimumLuma = UInt8.max
    var maximumLuma = UInt8.min
    var lumaTotal: UInt64 = 0

    for row in 0..<verticalSamples {
      let y = min(height - 1, row * height / verticalSamples)
      for column in 0..<horizontalSamples {
        let x = min(width - 1, column * width / horizontalSamples)
        let offset = y * bytesPerRow + x * 4
        let blue = bytes[offset]
        let green = bytes[offset + 1]
        let red = bytes[offset + 2]
        let luma = UInt8(
          (29 * UInt16(blue) + 150 * UInt16(green) + 77 * UInt16(red)) >> 8
        )

        minimumLuma = min(minimumLuma, luma)
        maximumLuma = max(maximumLuma, luma)
        lumaTotal += UInt64(luma)
        signature ^= UInt64(red) << 16 | UInt64(green) << 8 | UInt64(blue)
        signature &*= 1_099_511_628_211
      }
    }

    let sampleCount = horizontalSamples * verticalSamples
    return (
      signature: signature,
      minimumLuma: minimumLuma,
      maximumLuma: maximumLuma,
      averageLuma: Double(lumaTotal) / Double(sampleCount)
    )
  }

  private func recordAudio(_ sampleBuffer: CMSampleBuffer) {
    let presentationTime = sampleBuffer.presentationTimeStamp.seconds
    let frameCount = sampleBuffer.numSamples
    let audioDescription = sampleBuffer.formatDescription
      .flatMap(CMAudioFormatDescriptionGetStreamBasicDescription)
      .map { $0.pointee }
    let peak = audioDescription.map { audioPeak(in: sampleBuffer, format: $0) } ?? 0

    lock.withLock {
      audioBuffers += 1
      audioFrames += frameCount
      firstAudioPTS = firstAudioPTS ?? presentationTime
      lastAudioPTS = presentationTime
      if let audioDescription {
        audioSampleRate = audioDescription.mSampleRate
        audioChannels = audioDescription.mChannelsPerFrame
      }
      audioPeak = max(audioPeak, peak)
    }
  }

  private func frameStatus(for sampleBuffer: CMSampleBuffer) -> SCFrameStatus? {
    guard
      let attachmentsArray = CMSampleBufferGetSampleAttachmentsArray(
        sampleBuffer,
        createIfNecessary: false
      ) as? [[SCStreamFrameInfo: Any]],
      let attachments = attachmentsArray.first,
      let rawStatus = attachments[.status] as? Int
    else { return nil }
    return SCFrameStatus(rawValue: rawStatus)
  }

  private func audioPeak(
    in sampleBuffer: CMSampleBuffer,
    format: AudioStreamBasicDescription
  ) -> Float {
    guard
      format.mFormatID == kAudioFormatLinearPCM,
      format.mBitsPerChannel == 32,
      format.mFormatFlags & kAudioFormatFlagIsFloat != 0
    else { return 0 }

    var requiredSize = 0
    var blockBuffer: CMBlockBuffer?
    let sizeStatus = CMSampleBufferGetAudioBufferListWithRetainedBlockBuffer(
      sampleBuffer,
      bufferListSizeNeededOut: &requiredSize,
      bufferListOut: nil,
      bufferListSize: 0,
      blockBufferAllocator: nil,
      blockBufferMemoryAllocator: nil,
      flags: 0,
      blockBufferOut: &blockBuffer
    )
    guard sizeStatus == noErr, requiredSize > 0 else { return 0 }

    let storage = UnsafeMutableRawPointer.allocate(
      byteCount: requiredSize,
      alignment: MemoryLayout<AudioBufferList>.alignment
    )
    defer { storage.deallocate() }
    let bufferList = storage.bindMemory(to: AudioBufferList.self, capacity: 1)

    let listStatus = CMSampleBufferGetAudioBufferListWithRetainedBlockBuffer(
      sampleBuffer,
      bufferListSizeNeededOut: nil,
      bufferListOut: bufferList,
      bufferListSize: requiredSize,
      blockBufferAllocator: nil,
      blockBufferMemoryAllocator: nil,
      flags: 0,
      blockBufferOut: &blockBuffer
    )
    guard listStatus == noErr else { return 0 }

    var peak: Float = 0
    for buffer in UnsafeMutableAudioBufferListPointer(bufferList) {
      guard let data = buffer.mData else { continue }
      let sampleCount = Int(buffer.mDataByteSize) / MemoryLayout<Float>.size
      let samples = data.bindMemory(to: Float.self, capacity: sampleCount)
      for index in 0..<sampleCount {
        peak = max(peak, abs(samples[index]))
      }
    }
    return peak
  }
}

@main
@MainActor
private struct CaptureProbe {
  static func main() async {
    do {
      let command = try Command.parse(CommandLine.arguments.dropFirst())
      if case .help = command {
        print(usage)
        return
      }
      NSApplication.shared.setActivationPolicy(.prohibited)
      let content = try await SCShareableContent.excludingDesktopWindows(
        false,
        onScreenWindowsOnly: false
      )

      switch command {
      case .help:
        break
      case .list:
        list(content)
      case .listJSON:
        try listJSON(content)
      case .capture(let options):
        try await capture(content, options: options)
      case .publish(let options):
        try await publish(options: options)
      }
    } catch {
      FileHandle.standardError.write(Data("error: \(error.localizedDescription)\n".utf8))
      exit(EXIT_FAILURE)
    }
  }

  private static func list(_ content: SCShareableContent) {
    print("Displays:")
    for display in content.displays.sorted(by: { $0.displayID < $1.displayID }) {
      print("  id=\(display.displayID) pixels=\(display.width)x\(display.height)")
    }

    print("\nShareable windows:")
    for window in shareableWindows(content) {
      print(
        "  id=\(window.windowID) app=\(quoted(window.applicationName)) "
          + "title=\(quoted(window.title)) bundle=\(quoted(window.bundleIdentifier)) "
          + "points=\(window.width)x\(window.height) "
          + "capture-with=\(quoted("--window \(window.windowID)"))"
      )
    }
  }

  private static func listJSON(_ content: SCShareableContent) throws {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys]
    let data = try encoder.encode(SourceList(windows: shareableWindows(content)))
    FileHandle.standardOutput.write(data)
    FileHandle.standardOutput.write(Data("\n".utf8))
  }

  private static func shareableWindows(_ content: SCShareableContent) -> [WindowSource] {
    content.windows
      .filter { window in
        window.isOnScreen && window.windowLayer == 0 && window.owningApplication != nil
          && window.frame.width >= 320 && window.frame.height >= 180
      }
      .map { window in
        WindowSource(
          windowID: window.windowID,
          applicationName: window.owningApplication?.applicationName ?? "Unknown application",
          title: window.title?.trimmingCharacters(in: .whitespacesAndNewlines) ?? "",
          bundleIdentifier: window.owningApplication?.bundleIdentifier ?? "unknown",
          width: Int(window.frame.width),
          height: Int(window.frame.height)
        )
      }
      .sorted { lhs, rhs in
        let applicationOrder = lhs.applicationName.localizedCaseInsensitiveCompare(
          rhs.applicationName
        )
        if applicationOrder != .orderedSame {
          return applicationOrder == .orderedAscending
        }
        return lhs.title.localizedCaseInsensitiveCompare(rhs.title) == .orderedAscending
      }
  }

  private static func quoted(_ value: String) -> String {
    guard
      let data = try? JSONEncoder().encode(value),
      let encoded = String(data: data, encoding: .utf8)
    else { return "\"\"" }
    return encoded
  }

  private static func capture(
    _ content: SCShareableContent,
    options: CaptureOptions
  ) async throws {
    let prepared = try prepare(content, source: options.source)
    let outputSize = scaleToFit(
      width: prepared.sourceWidth,
      height: prepared.sourceHeight,
      maximumWidth: options.maximumWidth,
      maximumHeight: options.maximumHeight
    )

    let configuration = SCStreamConfiguration()
    configuration.width = outputSize.width
    configuration.height = outputSize.height
    configuration.minimumFrameInterval = CMTime(
      value: 1,
      timescale: CMTimeScale(options.frameRate)
    )
    configuration.queueDepth = 5
    configuration.pixelFormat = kCVPixelFormatType_32BGRA
    configuration.showsCursor = false
    configuration.capturesAudio = true
    configuration.sampleRate = 48_000
    configuration.channelCount = 2
    configuration.excludesCurrentProcessAudio = true

    let collector = SampleCollector(expectedFrameRate: options.frameRate)
    let stream = SCStream(
      filter: prepared.filter,
      configuration: configuration,
      delegate: collector
    )
    try stream.addStreamOutput(
      collector,
      type: .screen,
      sampleHandlerQueue: DispatchQueue(label: "chatto.capture-probe.video", qos: .userInteractive)
    )
    try stream.addStreamOutput(
      collector,
      type: .audio,
      sampleHandlerQueue: DispatchQueue(label: "chatto.capture-probe.audio", qos: .userInteractive)
    )
    let recordingOutput = try options.outputURL.map { outputURL in
      try prepareRecordingOutput(outputURL: outputURL, collector: collector, stream: stream)
    }

    print("Source: \(prepared.description)")
    print(
      "Requested: \(outputSize.width)x\(outputSize.height) @ \(options.frameRate) fps, "
        + "48 kHz stereo, \(String(format: "%.1f", options.duration)) seconds"
    )
    print(
      "Starting capture. The first run may trigger macOS Screen & System Audio Recording permission."
    )
    if let outputURL = options.outputURL {
      print("Recording: \(outputURL.path)")
    }

    try await stream.startCapture()
    try await Task.sleep(nanoseconds: UInt64(options.duration * 1_000_000_000))
    try await stream.stopCapture()

    printSummary(
      collector.snapshot(),
      outputURL: options.outputURL,
      recordedFileSize: recordingOutput?.recordedFileSize
    )
  }

  private static func publish(options: CaptureOptions) async throws {
    guard case .window(let windowID) = options.source else {
      throw ProbeError.invalidArguments("The publish command currently requires --window.")
    }
    try await LiveKitGamePublisher.run(
      windowID: windowID,
      frameRate: options.frameRate,
      maximumWidth: options.maximumWidth,
      maximumHeight: options.maximumHeight
    )
  }

  private static func prepareRecordingOutput(
    outputURL: URL,
    collector: SampleCollector,
    stream: SCStream
  ) throws -> SCRecordingOutput {
    guard outputURL.pathExtension.lowercased() == "mov" else {
      throw ProbeError.invalidArguments("--output must use a .mov file extension.")
    }
    guard !FileManager.default.fileExists(atPath: outputURL.path) else {
      throw ProbeError.invalidArguments("Refusing to overwrite existing file: \(outputURL.path)")
    }
    try FileManager.default.createDirectory(
      at: outputURL.deletingLastPathComponent(),
      withIntermediateDirectories: true
    )

    let configuration = SCRecordingOutputConfiguration()
    guard configuration.availableOutputFileTypes.contains(.mov) else {
      throw ProbeError.invalidArguments("ScreenCaptureKit does not support QuickTime output here.")
    }
    configuration.outputURL = outputURL
    configuration.outputFileType = .mov
    let output = SCRecordingOutput(configuration: configuration, delegate: collector)
    try stream.addRecordingOutput(output)
    return output
  }

  private static func prepare(
    _ content: SCShareableContent,
    source: CaptureOptions.Source
  ) throws -> PreparedCapture {
    switch source {
    case .display(let displayID):
      guard let display = content.displays.first(where: { $0.displayID == displayID }) else {
        throw ProbeError.sourceNotFound("Display \(displayID) is not available. Run list again.")
      }
      return PreparedCapture(
        description: "display \(displayID)",
        filter: SCContentFilter(
          display: display,
          excludingApplications: [],
          exceptingWindows: []
        ),
        sourceWidth: display.width,
        sourceHeight: display.height
      )

    case .window(let windowID):
      guard let window = content.windows.first(where: { $0.windowID == windowID }) else {
        throw ProbeError.sourceNotFound("Window \(windowID) is not available. Run list again.")
      }
      let scale = displayScale(for: window, displays: content.displays)
      return PreparedCapture(
        description: "window \(windowID) owned by "
          + (window.owningApplication?.bundleIdentifier ?? "unknown"),
        filter: SCContentFilter(desktopIndependentWindow: window),
        sourceWidth: max(1, Int((window.frame.width * scale).rounded())),
        sourceHeight: max(1, Int((window.frame.height * scale).rounded()))
      )

    case .application(let bundleIdentifier, let requestedDisplayID):
      guard
        let application = content.applications.first(where: {
          $0.bundleIdentifier == bundleIdentifier
        })
      else {
        throw ProbeError.sourceNotFound(
          "Application \(bundleIdentifier) is not available. Run list again."
        )
      }
      let display: SCDisplay
      if let requestedDisplayID {
        guard
          let requested = content.displays.first(where: {
            $0.displayID == requestedDisplayID
          })
        else {
          throw ProbeError.sourceNotFound(
            "Display \(requestedDisplayID) is not available. Run list again."
          )
        }
        display = requested
      } else if content.displays.count == 1, let onlyDisplay = content.displays.first {
        display = onlyDisplay
      } else {
        throw ProbeError.invalidArguments(
          "--application requires --on-display when more than one display is connected."
        )
      }
      return PreparedCapture(
        description: "application \(bundleIdentifier) on display \(display.displayID)",
        filter: SCContentFilter(
          display: display,
          including: [application],
          exceptingWindows: []
        ),
        sourceWidth: display.width,
        sourceHeight: display.height
      )
    }
  }

  private static func displayScale(for window: SCWindow, displays: [SCDisplay]) -> CGFloat {
    let display = displays.max { lhs, rhs in
      lhs.frame.intersection(window.frame).area < rhs.frame.intersection(window.frame).area
    }
    guard let display, display.frame.width > 0 else { return 1 }
    return CGFloat(display.width) / display.frame.width
  }

  private static func scaleToFit(
    width: Int,
    height: Int,
    maximumWidth: Int,
    maximumHeight: Int
  ) -> (width: Int, height: Int) {
    let scale = min(
      1,
      Double(maximumWidth) / Double(width),
      Double(maximumHeight) / Double(height)
    )
    return (
      width: max(2, Int((Double(width) * scale).rounded()) & ~1),
      height: max(2, Int((Double(height) * scale).rounded()) & ~1)
    )
  }

  private static func printSummary(
    _ snapshot: MetricsSnapshot,
    outputURL: URL?,
    recordedFileSize: Int?
  ) {
    print("\nCapture summary:")
    print(
      "  video: \(snapshot.completeVideoFrames) complete / "
        + "\(snapshot.totalVideoBuffers) buffers, "
        + "\(snapshot.videoWidth)x\(snapshot.videoHeight), "
        + "\(String(format: "%.2f", snapshot.videoFramesPerSecond)) fps"
    )
    print(
      "  video diagnostics: \(snapshot.nonCompleteVideoBuffers) non-complete buffers, "
        + "\(snapshot.inferredVideoGaps) inferred missing frame intervals"
    )
    print(
      "  video content: \(snapshot.changedContentSamples)/"
        + "\(max(0, snapshot.contentSamples - 1)) sampled frames changed, "
        + "luma min/mean/max=\(snapshot.minimumLuma)/"
        + "\(String(format: "%.1f", snapshot.averageLuma))/\(snapshot.maximumLuma)"
    )
    print(
      "  audio: \(snapshot.audioBuffers) buffers, \(snapshot.audioFrames) frames, "
        + "\(Int(snapshot.audioSampleRate)) Hz, \(snapshot.audioChannels) channels, "
        + "peak=\(String(format: "%.4f", snapshot.audioPeak)), "
        + "PTS span=\(String(format: "%.3f", snapshot.audioDuration))s"
    )
    if let stopError = snapshot.stopError {
      print("  stream error: \(stopError)")
    }
    if let outputURL {
      print(
        "  recording: started=\(snapshot.recordingStarted), "
          + "finished=\(snapshot.recordingFinished), "
          + "bytes=\(recordedFileSize ?? 0), file=\(outputURL.path)"
      )
      if let recordingError = snapshot.recordingError {
        print("  recording error: \(recordingError)")
      }
    }

    if snapshot.completeVideoFrames == 0 {
      print("  result: FAIL — no complete video frames arrived")
    } else if snapshot.contentSamples == 0 {
      print("  result: INCONCLUSIVE — video arrived but its pixel format could not be inspected")
    } else if snapshot.maximumLuma < 8 {
      print("  result: FAIL — video arrived but sampled pixels were effectively black")
    } else if snapshot.contentSamples > 1, snapshot.changedContentSamples == 0 {
      print("  result: FAIL — video arrived but sampled pixels never changed")
    } else if outputURL != nil, snapshot.recordingError != nil {
      print("  result: FAIL — media arrived but recording failed")
    } else if snapshot.audioBuffers == 0 {
      print("  result: FAIL — no audio buffers arrived")
    } else if snapshot.audioPeak == 0 {
      print("  result: INCONCLUSIVE — audio arrived but was silent or not 32-bit float PCM")
    } else {
      print("  result: PASS — video frames and non-silent audio arrived")
    }
  }
}

extension CGRect {
  fileprivate var area: CGFloat {
    guard !isNull else { return 0 }
    return width * height
  }
}
