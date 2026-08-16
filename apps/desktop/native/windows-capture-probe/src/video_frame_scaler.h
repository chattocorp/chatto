// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#pragma once

#include <algorithm>
#include <cstdint>
#include <stdexcept>
#include <utility>
#include <vector>

namespace chatto::capture {

/** Scale a tightly packed BGRA frame to the track's stable output dimensions.
 */
[[nodiscard]] inline std::vector<std::uint8_t> scale_bgra_frame(
    std::vector<std::uint8_t> pixels, const std::uint32_t source_width,
    const std::uint32_t source_height, const std::uint32_t output_width,
    const std::uint32_t output_height) {
  const auto source_bytes =
      static_cast<std::size_t>(source_width) * source_height * 4;
  if (source_width == 0 || source_height == 0 || output_width == 0 ||
      output_height == 0 || pixels.size() != source_bytes) {
    throw std::invalid_argument("The BGRA frame dimensions are invalid");
  }
  if (source_width == output_width && source_height == output_height) {
    return pixels;
  }

  std::vector<std::uint8_t> output(static_cast<std::size_t>(output_width) *
                                   output_height * 4);
  for (std::uint32_t y = 0; y < output_height; ++y) {
    const auto source_y = static_cast<std::uint32_t>(
        static_cast<std::uint64_t>(y) * source_height / output_height);
    for (std::uint32_t x = 0; x < output_width; ++x) {
      const auto source_x = static_cast<std::uint32_t>(
          static_cast<std::uint64_t>(x) * source_width / output_width);
      const auto source_offset =
          (static_cast<std::size_t>(source_y) * source_width + source_x) * 4;
      const auto output_offset =
          (static_cast<std::size_t>(y) * output_width + x) * 4;
      std::copy_n(pixels.data() + source_offset, 4,
                  output.data() + output_offset);
    }
  }
  return output;
}

} // namespace chatto::capture
