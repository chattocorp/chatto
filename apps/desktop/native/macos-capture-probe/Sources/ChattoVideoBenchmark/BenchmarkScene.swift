// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import CoreGraphics
import CoreText
import CoreVideo
import Foundation
import ImageIO
import VideoToolbox

/// A fixed synthetic scene with textured tiles, edges, text, and moving objects.
/// All variants receive identical BGRA pixels and timestamps. This is a stress
/// fixture, not a representative recording or a claim about game quality.
final class BenchmarkScene {
  let width: Int
  let height: Int
  let fps: Int
  private let world: CGImage
  static let bitmapInfo = CGBitmapInfo.byteOrder32Little.rawValue | CGImageAlphaInfo.premultipliedFirst.rawValue

  init(width: Int, height: Int, fps: Int) throws {
    self.width = width
    self.height = height
    self.fps = fps
    let worldWidth = width * 2
    let worldHeight = height * 2
    guard let context = CGContext(
      data: nil, width: worldWidth, height: worldHeight, bitsPerComponent: 8,
      bytesPerRow: worldWidth * 4, space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: Self.bitmapInfo
    ), let data = context.data?.assumingMemoryBound(to: UInt8.self) else {
      throw BenchmarkError(message: "Cannot allocate scene.")
    }
    // Fixed integer hash: grain and tile colors do not vary between invocations.
    for y in 0..<worldHeight {
      for x in 0..<worldWidth {
        let tile = UInt32((x / 48) + (y / 48) * 131) &* 2_654_435_761
        var hash = UInt32((x / 2) + (y / 2) * worldWidth) &+ 1
        hash = (hash ^ (hash >> 16)) &* 2_246_822_519
        hash = (hash ^ (hash >> 13)) &* 3_266_489_917
        let grain = Int((hash ^ (hash >> 16)) & 31)
        let offset = (y * worldWidth + x) * 4
        data[offset] = UInt8(35 + Int(tile & 31) + grain)
        data[offset + 1] = UInt8(65 + Int((tile >> 8) & 63) + grain)
        data[offset + 2] = UInt8(30 + Int((tile >> 16) & 31) + grain)
        data[offset + 3] = 255
      }
    }
    for y in stride(from: 0, to: worldHeight, by: 144) {
      for x in stride(from: 0, to: worldWidth, by: 192) {
        context.setFillColor(CGColor(red: 0.12, green: 0.16, blue: 0.2, alpha: 1))
        context.fill(CGRect(x: x + 8, y: y + 8, width: 108, height: 70))
        context.setStrokeColor(CGColor(gray: 0.8, alpha: 1))
        context.stroke(CGRect(x: x + 8, y: y + 8, width: 108, height: 70), width: 1)
        Self.text("TILE \(x / 192):\(y / 144)", at: CGPoint(x: x + 15, y: y + 48), size: 13, context: context)
        Self.text("012345 / AaBb", at: CGPoint(x: x + 15, y: y + 25), size: 11, context: context)
      }
    }
    guard let image = context.makeImage() else { throw BenchmarkError(message: "Cannot render scene.") }
    world = image
  }

  func frame(_ index: Int) throws -> CVPixelBuffer {
    var pixelBuffer: CVPixelBuffer?
    let status = CVPixelBufferCreate(
      kCFAllocatorDefault, width, height, kCVPixelFormatType_32BGRA,
      [kCVPixelBufferCGImageCompatibilityKey: true, kCVPixelBufferCGBitmapContextCompatibilityKey: true,
       kCVPixelBufferIOSurfacePropertiesKey: [:]] as CFDictionary, &pixelBuffer
    )
    guard status == kCVReturnSuccess, let pixelBuffer else { throw BenchmarkError(message: "Cannot allocate frame: \(status)") }
    CVPixelBufferLockBaseAddress(pixelBuffer, [])
    defer { CVPixelBufferUnlockBaseAddress(pixelBuffer, []) }
    guard let context = CGContext(
      data: CVPixelBufferGetBaseAddress(pixelBuffer), width: width, height: height, bitsPerComponent: 8,
      bytesPerRow: CVPixelBufferGetBytesPerRow(pixelBuffer), space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: Self.bitmapInfo
    ) else { throw BenchmarkError(message: "Cannot create frame context.") }
    let time = Double(index) / Double(fps)
    let distance = time < 2 ? 0 : (time < 4 ? (time - 2) * 100 : 200 + (time - 4) * 550)
    let x = Int(distance) % width
    let y = Int(distance * 0.31) % height
    context.interpolationQuality = .none
    context.draw(world, in: CGRect(x: -x, y: -y, width: width * 2, height: height * 2))
    if time >= 2 {
      for object in 0..<16 {
        let px = (index * (object + 2) + object * 117) % width
        let py = (object * 71 + index * 2) % height
        context.setFillColor(CGColor(red: 0.95, green: CGFloat(object % 3) / 3, blue: 0.15, alpha: 1))
        context.fillEllipse(in: CGRect(x: px, y: py, width: 12, height: 12))
      }
    }
    context.setFillColor(CGColor(gray: 0.04, alpha: 0.95))
    context.fill(CGRect(x: 0, y: 0, width: width, height: 70))
    Self.text("Chatto encoder fixture | HP 100/100 | Inventory: 0123456789", at: CGPoint(x: 16, y: 42), size: 18, context: context)
    Self.text("Still 0-2s / slow pan 2-4s / fast pan 4-6s", at: CGPoint(x: 16, y: 16), size: 14, context: context)
    return pixelBuffer
  }

  static func text(_ text: String, at point: CGPoint, size: CGFloat, context: CGContext) {
    let attributes: [NSAttributedString.Key: Any] = [
      NSAttributedString.Key(kCTFontAttributeName as String): CTFontCreateWithName("Menlo" as CFString, size, nil),
      NSAttributedString.Key(kCTForegroundColorAttributeName as String): CGColor(gray: 1, alpha: 1),
    ]
    context.textPosition = point
    CTLineDraw(CTLineCreateWithAttributedString(NSAttributedString(string: text, attributes: attributes)), context)
  }

  static func image(_ buffer: CVPixelBuffer) throws -> CGImage {
    var image: CGImage?
    let status = VTCreateCGImageFromCVPixelBuffer(buffer, options: nil, imageOut: &image)
    guard status == noErr, let image else { throw BenchmarkError(message: "Cannot convert image: \(status)") }
    return image
  }

  static func savePNG(_ image: CGImage, to url: URL) throws {
    guard let destination = CGImageDestinationCreateWithURL(url as CFURL, "public.png" as CFString, 1, nil) else {
      throw BenchmarkError(message: "Cannot create PNG.")
    }
    CGImageDestinationAddImage(destination, image, nil)
    guard CGImageDestinationFinalize(destination) else { throw BenchmarkError(message: "Cannot write PNG.") }
  }

  /// Full-frame RGB-weighted luma error. Conversion
  /// to/from 4:2:0 contributes error too; compare variants, not absolute scores.
  static func lumaMSE(_ reference: CVPixelBuffer, _ decoded: CVPixelBuffer) throws -> Double {
    guard CVPixelBufferGetWidth(reference) == CVPixelBufferGetWidth(decoded),
      CVPixelBufferGetHeight(reference) == CVPixelBufferGetHeight(decoded),
      CVPixelBufferGetPixelFormatType(reference) == kCVPixelFormatType_32BGRA,
      CVPixelBufferGetPixelFormatType(decoded) == kCVPixelFormatType_32BGRA else {
      throw BenchmarkError(message: "Quality comparison requires equal BGRA dimensions.")
    }
    CVPixelBufferLockBaseAddress(reference, .readOnly)
    CVPixelBufferLockBaseAddress(decoded, .readOnly)
    defer {
      CVPixelBufferUnlockBaseAddress(reference, .readOnly)
      CVPixelBufferUnlockBaseAddress(decoded, .readOnly)
    }
    let a = CVPixelBufferGetBaseAddress(reference)!.assumingMemoryBound(to: UInt8.self)
    let b = CVPixelBufferGetBaseAddress(decoded)!.assumingMemoryBound(to: UInt8.self)
    var total = 0.0
    var count = 0
    for y in 0..<CVPixelBufferGetHeight(reference) {
      for x in 0..<CVPixelBufferGetWidth(reference) {
        let ai = y * CVPixelBufferGetBytesPerRow(reference) + x * 4
        let bi = y * CVPixelBufferGetBytesPerRow(decoded) + x * 4
        let difference = 0.0722 * Double(Int(a[ai]) - Int(b[bi]))
          + 0.7152 * Double(Int(a[ai + 1]) - Int(b[bi + 1]))
          + 0.2126 * Double(Int(a[ai + 2]) - Int(b[bi + 2]))
        total += difference * difference
        count += 1
      }
    }
    return total / Double(count)
  }
}
