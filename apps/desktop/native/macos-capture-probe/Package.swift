// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

// swift-tools-version: 6.1

import PackageDescription

let package = Package(
  name: "chatto-macos-capture-probe",
  platforms: [.macOS(.v15)],
  products: [
    .executable(
      name: "chatto-macos-capture-probe",
      targets: ["ChattoMacOSCaptureProbe"]
    ),
    .executable(name: "chatto-video-benchmark", targets: ["ChattoVideoBenchmark"]),
  ],
  dependencies: [
    .package(url: "https://github.com/livekit/client-sdk-swift.git", exact: "2.18.0")
  ],
  targets: [
    .executableTarget(name: "ChattoVideoBenchmark"),
    .testTarget(name: "ChattoVideoBenchmarkTests", dependencies: ["ChattoVideoBenchmark"]),
    .executableTarget(
      name: "ChattoMacOSCaptureProbe",
      dependencies: [.product(name: "LiveKit", package: "client-sdk-swift")]
    ),
    .testTarget(
      name: "ChattoMacOSCaptureProbeTests",
      dependencies: [
        "ChattoMacOSCaptureProbe",
        .product(name: "LiveKit", package: "client-sdk-swift"),
      ]
    ),
  ]
)
