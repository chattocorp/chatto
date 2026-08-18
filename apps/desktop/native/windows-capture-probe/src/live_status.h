// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#pragma once

#include <atomic>
#include <cstdint>

namespace chatto::capture {

// Cross-thread latest-value diagnostics for the optional local preview.
struct LiveCaptureStatus {
  std::atomic<std::uint64_t> audio_frames = 0;
  std::atomic<std::uint64_t> audio_discontinuities = 0;
  std::atomic<float> latest_audio_peak = 0;
};

}  // namespace chatto::capture
