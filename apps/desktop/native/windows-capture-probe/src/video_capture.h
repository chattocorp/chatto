// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#pragma once

#include <chrono>
#include <cstdint>
#include <functional>
#include <stop_token>
#include <string>
#include <vector>

#include <memory>
#include <utility>

#include <windows.h>

namespace chatto::capture {

struct LiveCaptureStatus;

struct VideoFrameData {
  std::uint32_t width;
  std::uint32_t height;
  std::int64_t timestamp_100ns;
  double readback_duration_ms;
  std::vector<std::uint8_t> bgra;
};

using VideoFrameHandler = std::function<void(VideoFrameData)>;

[[nodiscard]] std::pair<std::uint32_t, std::uint32_t>
window_capture_size(HWND window);

struct VideoCaptureMetrics {
  std::uint64_t frames = 0;
  std::uint64_t inferred_gaps = 0;
  std::uint64_t resizes = 0;
  std::uint64_t sampled_frames = 0;
  std::uint64_t changed_samples = 0;
  std::uint64_t black_samples = 0;
  std::uint32_t width = 0;
  std::uint32_t height = 0;
  std::int64_t first_timestamp_100ns = 0;
  std::int64_t last_timestamp_100ns = 0;
  double timestamp_span_seconds = 0;
  double observed_frames_per_second = 0;
  double longest_frame_interval_ms = 0;
  double sampled_luminance_mean = 0;
  std::uint8_t sampled_luminance_min = 0;
  std::uint8_t sampled_luminance_max = 0;
  double wall_duration_seconds = 0;
  double process_cpu_seconds = 0;
  double process_cpu_single_core_percent = 0;
  std::uint64_t peak_working_set_bytes = 0;
  bool source_closed = false;
  bool frame_stalled = false;
  bool presentation_changed = false;
  bool stop_requested = false;
  std::int32_t error_code = 0;
  std::wstring error;
};

/** Whether the selected foreground window currently covers its monitor. */
[[nodiscard]] bool is_foreground_monitor_covering_window(HWND window);

[[nodiscard]] VideoCaptureMetrics capture_window_video(
    HWND window, std::chrono::seconds duration,
    std::uint32_t requested_frames_per_second, bool show_preview,
    std::shared_ptr<LiveCaptureStatus> live_status = {},
    VideoFrameHandler frame_handler = {}, std::stop_token stop_token = {},
    std::chrono::milliseconds frame_stall_timeout = {},
    bool switch_on_monitor_covering_presentation = false);

/**
 * Capture a foreground monitor-covering window through monitor WGC.
 *
 * The call returns with `presentation_changed` when the window leaves that
 * presentation mode, allowing the caller to resume privacy-preserving WGC.
 */
[[nodiscard]] VideoCaptureMetrics capture_monitor_covering_window_wgc_video(
    HWND window, std::chrono::seconds duration,
    std::uint32_t requested_frames_per_second,
    VideoFrameHandler frame_handler = {}, std::stop_token stop_token = {},
    std::chrono::milliseconds frame_stall_timeout = {});

/** Capture an explicitly selected monitor through Windows Graphics Capture. */
[[nodiscard]] VideoCaptureMetrics capture_monitor_wgc_video(
    HMONITOR monitor, std::chrono::seconds duration,
    std::uint32_t requested_frames_per_second,
    VideoFrameHandler frame_handler = {}, std::stop_token stop_token = {},
    std::chrono::milliseconds frame_stall_timeout = {});

/** Capture a foreground monitor-covering window through Desktop Duplication. */
[[nodiscard]] VideoCaptureMetrics capture_monitor_covering_window_dxgi_video(
    HWND window, std::chrono::seconds duration,
    std::uint32_t requested_frames_per_second,
    VideoFrameHandler frame_handler = {}, std::stop_token stop_token = {});

} // namespace chatto::capture
