// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#pragma once

#include <chrono>
#include <cstdint>
#include <functional>
#include <stop_token>

#include <memory>

#include <windows.h>

namespace chatto::capture {

struct LiveCaptureStatus;

struct AudioFrameData {
  std::uint32_t sample_rate;
  std::uint16_t channels;
  std::uint32_t frames;
  std::uint64_t timestamp_100ns;
  const float* samples;
  bool silent;
};

using AudioFrameHandler = std::function<void(const AudioFrameData&)>;

struct AudioCaptureMetrics {
  std::uint64_t packets = 0;
  std::uint64_t frames = 0;
  std::uint64_t silent_packets = 0;
  std::uint64_t discontinuities = 0;
  std::uint64_t timestamp_errors = 0;
  std::uint32_t sample_rate = 0;
  std::uint16_t channels = 0;
  std::uint64_t first_timestamp_100ns = 0;
  std::uint64_t last_timestamp_100ns = 0;
  double timestamp_span_seconds = 0;
  float peak_level = 0;
};

[[nodiscard]] AudioCaptureMetrics capture_process_audio(
    DWORD process_id,
    std::chrono::seconds duration,
    std::stop_token stop_token = {},
    std::shared_ptr<LiveCaptureStatus> live_status = {},
    AudioFrameHandler frame_handler = {});

}  // namespace chatto::capture
