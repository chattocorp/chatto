// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import CoreVideo
import Testing
@testable import ChattoVideoBenchmark

@Test
func benchmarkWindowsUseMediaTimeIncludingDroppedFrameGaps() {
  let frames = [
    EncodedFrameMeasurement(frame: 0, time: 0, bytes: 100, latencyMs: 1),
    EncodedFrameMeasurement(frame: 1, time: 0.5, bytes: 200, latencyMs: 1),
    EncodedFrameMeasurement(frame: 2, time: 1, bytes: 300, latencyMs: 1),
    EncodedFrameMeasurement(frame: 3, time: 4, bytes: 400, latencyMs: 1),
  ]
  #expect(BenchmarkMath.peakBps(frames, seconds: 1) == 4000)
  #expect(BenchmarkMath.peakBps(Array(frames.reversed()), seconds: 5) == 1600)
  #expect(BenchmarkMath.peakBps([], seconds: 1) == 0)
}

@Test
func benchmarkSceneIsRepeatableAndMotionChangesPixels() throws {
  let scene = try BenchmarkScene(width: 192, height: 128, fps: 60)
  let sameScene = try BenchmarkScene(width: 192, height: 128, fps: 60)
  let first = try scene.frame(0)
  #expect(try BenchmarkScene.lumaMSE(first, scene.frame(60)) == 0)
  #expect(try BenchmarkScene.lumaMSE(scene.frame(150), sameScene.frame(150)) == 0)
  #expect(try BenchmarkScene.lumaMSE(first, scene.frame(150)) > 0)
}

@Test
func benchmarkConfigurationRejectsInvalidInputAndKeepsEightMbpsCeiling() throws {
  for arguments in [
    ["--output", "out", "--width", "101"],
    ["--output", "out", "--fps", "0"],
    ["--output", "out", "--only", "bad"],
    ["--output", "out", "--only", "baseline-6,baseline-6"],
    ["--output", "out", "--repeats", "100"],
    ["--output", "out", "--output", "other"],
  ] {
    #expect(throws: BenchmarkError.self) { try BenchmarkOptions.parse(arguments) }
  }
  let options = try BenchmarkOptions.parse(["--output", "out", "--only", "baseline-6,quality-6"])
  #expect(options.configurations.map(\.name) == ["baseline-6", "quality-6"])
  for configuration in EncoderConfiguration.matrix {
    #expect(configuration.targetBps <= configuration.ceilingBps)
    #expect(configuration.ceilingBps <= 8_000_000)
    #expect(!configuration.lowLatency || configuration.highProfile)
  }
}
