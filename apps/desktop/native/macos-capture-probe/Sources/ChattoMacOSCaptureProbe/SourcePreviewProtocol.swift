// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import Foundation

/// Metadata for one native source and the JPEG bytes that immediately follow
/// the manifest in the source-preview wire frame.
struct SourcePreviewRecord: Codable, Equatable, Sendable {
  enum Kind: String, Codable, Sendable {
    case display
    case window
  }

  let kind: Kind
  let nativeID: UInt32
  let applicationName: String?
  let bundleIdentifier: String?
  let title: String
  let width: Int
  let height: Int
  let displayIndex: Int?
  let isMainDisplay: Bool?
  let previewByteLength: Int
}

struct SourcePreviewManifest: Codable, Equatable, Sendable {
  let protocolVersion: Int
  let sources: [SourcePreviewRecord]

  init(sources: [SourcePreviewRecord]) {
    protocolVersion = 1
    self.sources = sources
  }
}

/// A compact binary response avoids expanding sensitive preview images into
/// base64 JSON. The first four bytes are a big-endian JSON-manifest length;
/// JPEG payloads then follow in manifest order.
enum SourcePreviewProtocol {
  static func encode(manifest: SourcePreviewManifest, previews: [Data]) throws -> Data {
    guard manifest.sources.count == previews.count else {
      throw SourcePreviewProtocolError.previewCountMismatch
    }
    for (source, preview) in zip(manifest.sources, previews) {
      guard source.previewByteLength == preview.count else {
        throw SourcePreviewProtocolError.previewLengthMismatch
      }
    }

    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys]
    let manifestData = try encoder.encode(manifest)
    guard let manifestLength = UInt32(exactly: manifestData.count) else {
      throw SourcePreviewProtocolError.manifestTooLarge
    }

    var frame = Data(capacity: 4 + manifestData.count + previews.reduce(0) { $0 + $1.count })
    var bigEndianLength = manifestLength.bigEndian
    withUnsafeBytes(of: &bigEndianLength) { frame.append(contentsOf: $0) }
    frame.append(manifestData)
    for preview in previews { frame.append(preview) }
    return frame
  }
}

private enum SourcePreviewProtocolError: Error {
  case manifestTooLarge
  case previewCountMismatch
  case previewLengthMismatch
}
