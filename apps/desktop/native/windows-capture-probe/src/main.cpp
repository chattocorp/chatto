// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#include <algorithm>
#include <array>
#include <atomic>
#include <charconv>
#include <chrono>
#include <future>
#include <iomanip>
#include <iostream>
#include <iterator>
#include <limits>
#include <memory>
#include <string>
#include <string_view>
#include <thread>
#include <vector>

#include <io.h>
#include <windows.h>
#include <winrt/Windows.Data.Json.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.Graphics.Capture.h>
#include <winrt/base.h>

#include "audio_capture.h"
#include "live_status.h"
#include "livekit_publisher.h"
#include "video_capture.h"
#include "window_sources.h"

namespace {

class EncodedPreviewPipe final {
public:
  EncodedPreviewPipe() {
    const auto native_handle = _get_osfhandle(3);
    if (native_handle != -1) {
      handle_ = reinterpret_cast<HANDLE>(native_handle);
    }
  }

  void write(std::span<const std::uint8_t> data,
             const std::int64_t timestamp_us, const bool key_frame) {
    if (handle_ == INVALID_HANDLE_VALUE || data.empty() ||
        data.size() > 16U * 1024U * 1024U) {
      return;
    }
    std::array<std::uint8_t, 16> header{'C', 'T', 'P', 'V'};
    const auto size = static_cast<std::uint32_t>(data.size());
    for (std::size_t index = 0; index < 4; ++index) {
      header[4 + index] =
          static_cast<std::uint8_t>((size >> (index * 8U)) & 0xffU);
    }
    const auto timestamp = static_cast<std::uint64_t>(timestamp_us);
    for (std::size_t index = 0; index < 8; ++index) {
      header[8 + index] =
          static_cast<std::uint8_t>((timestamp >> (index * 8U)) & 0xffU);
    }
    if (key_frame) header[7] |= 0x80U;
    if (!write_all(header) || !write_all(data)) {
      handle_ = INVALID_HANDLE_VALUE;
    }
  }

private:
  bool write_all(std::span<const std::uint8_t> bytes) const {
    while (!bytes.empty()) {
      DWORD written = 0;
      if (!WriteFile(handle_, bytes.data(), static_cast<DWORD>(bytes.size()),
                     &written, nullptr) || written == 0) {
        return false;
      }
      bytes = bytes.subspan(written);
    }
    return true;
  }

  HANDLE handle_ = INVALID_HANDLE_VALUE;
};

void print_usage() {
  std::wcout
      << L"Usage:\n"
      << L"  chatto-windows-capture-probe support\n"
      << L"  chatto-windows-capture-probe list [--include-titles]\n"
      << L"  chatto-windows-capture-probe list-json [--exclude-process <pid>]\n"
      << L"  chatto-windows-capture-probe capture --window <hwnd> "
         L"[--duration <seconds>] [--fps <rate>] [--video-only] [--preview]\n"
      << L"  chatto-windows-capture-probe publish (--window <hwnd> "
         L"--expected-window-bundle <identifier> | --display <hmonitor>) "
         L"[--fps <rate>]\n"
      << L"  chatto-windows-capture-probe audio --process <pid> "
         L"[--duration <seconds>]\n\n"
      << L"Window titles may contain sensitive information and are omitted by "
         L"default.\n";
}

[[nodiscard]] std::uint64_t parse_unsigned(const std::wstring_view value,
                                           const int base, const char *name) {
  wchar_t *end = nullptr;
  errno = 0;
  const auto parsed = std::wcstoull(value.data(), &end, base);
  if (errno != 0 || end != value.data() + value.size()) {
    throw std::invalid_argument(std::string("Invalid ") + name);
  }
  return parsed;
}

int print_support() {
  const bool supported =
      winrt::Windows::Graphics::Capture::GraphicsCaptureSession::IsSupported();
  std::wcout << L"windows_graphics_capture="
             << (supported ? L"supported" : L"unsupported") << L"\n";
  return supported ? 0 : 2;
}

int list_windows(const bool include_titles) {
  const auto sources = chatto::capture::enumerate_window_sources();
  std::wcout << L"windows=" << sources.size() << L"\n";
  for (const auto &source : sources) {
    std::wcout << L"hwnd=0x" << std::hex
               << reinterpret_cast<std::uintptr_t>(source.handle) << std::dec
               << L" pid=" << source.process_id << L" application="
               << std::quoted(source.application_name) << L" size="
               << source.width << L"x" << source.height;
    if (include_titles) {
      std::wcout << L" title=" << std::quoted(source.title);
    }
    std::wcout << L"\n";
  }
  return 0;
}

int list_windows_json(const int argument_count, wchar_t *arguments[]) {
  DWORD excluded_process_id = 0;
  for (int index = 2; index < argument_count; ++index) {
    if (std::wstring_view(arguments[index]) == L"--exclude-process" &&
        index + 1 < argument_count) {
      const auto value =
          parse_unsigned(arguments[++index], 10, "process identifier");
      if (value > std::numeric_limits<DWORD>::max()) {
        throw std::invalid_argument("Process identifier is out of range");
      }
      excluded_process_id = static_cast<DWORD>(value);
      continue;
    }
    throw std::invalid_argument("Unknown or incomplete list-json argument");
  }

  using namespace winrt::Windows::Data::Json;
  JsonArray sources;
  for (const auto &source : chatto::capture::enumerate_display_sources()) {
    JsonObject value;
    value.SetNamedValue(L"kind", JsonValue::CreateStringValue(L"display"));
    value.SetNamedValue(L"nativeID",
                        JsonValue::CreateNumberValue(static_cast<double>(
                            reinterpret_cast<std::uintptr_t>(source.handle))));
    value.SetNamedValue(L"displayIndex",
                        JsonValue::CreateNumberValue(source.display_index));
    value.SetNamedValue(L"isMainDisplay",
                        JsonValue::CreateBooleanValue(source.is_main_display));
    value.SetNamedValue(L"width", JsonValue::CreateNumberValue(source.width));
    value.SetNamedValue(L"height", JsonValue::CreateNumberValue(source.height));
    value.SetNamedValue(L"previewByteLength", JsonValue::CreateNumberValue(0));
    sources.Append(value);
  }
  for (const auto &source : chatto::capture::enumerate_window_sources()) {
    if (source.process_id == excluded_process_id) {
      continue;
    }
    JsonObject value;
    value.SetNamedValue(L"kind", JsonValue::CreateStringValue(L"window"));
    value.SetNamedValue(L"nativeID",
                        JsonValue::CreateNumberValue(static_cast<double>(
                            reinterpret_cast<std::uintptr_t>(source.handle))));
    value.SetNamedValue(L"applicationName",
                        JsonValue::CreateStringValue(source.application_name));
    value.SetNamedValue(
        L"bundleIdentifier",
        JsonValue::CreateStringValue(source.application_identifier));
    value.SetNamedValue(L"title", JsonValue::CreateStringValue(source.title));
    value.SetNamedValue(L"width", JsonValue::CreateNumberValue(source.width));
    value.SetNamedValue(L"height", JsonValue::CreateNumberValue(source.height));
    value.SetNamedValue(L"previewByteLength", JsonValue::CreateNumberValue(0));
    sources.Append(value);
  }
  JsonObject response;
  response.SetNamedValue(L"protocolVersion", JsonValue::CreateNumberValue(1));
  response.SetNamedValue(L"sources", sources);
  std::cout << winrt::to_string(response.Stringify());
  return 0;
}

void print_audio_metrics(const chatto::capture::AudioCaptureMetrics &metrics) {
  std::wcout << std::fixed << std::setprecision(4) << L"audio_packets="
             << metrics.packets << L"\n"
             << L"audio_frames=" << metrics.frames << L"\n"
             << L"audio_format=" << metrics.sample_rate << L"Hz/"
             << metrics.channels << L"ch/float32\n"
             << L"audio_timestamp_span_seconds="
             << metrics.timestamp_span_seconds << L"\n"
             << L"audio_peak=" << metrics.peak_level << L"\n"
             << L"audio_silent_packets=" << metrics.silent_packets << L"\n"
             << L"audio_discontinuities=" << metrics.discontinuities << L"\n"
             << L"audio_timestamp_errors=" << metrics.timestamp_errors << L"\n";
}

int capture_window(const int argument_count, wchar_t *arguments[]) {
  HWND window = nullptr;
  std::uint32_t duration_seconds = 15;
  std::uint32_t frames_per_second = 60;
  bool video_only = false;
  bool show_preview = false;

  for (int index = 2; index < argument_count; ++index) {
    const std::wstring_view argument(arguments[index]);
    if (argument == L"--window" && index + 1 < argument_count) {
      const auto value = parse_unsigned(arguments[++index], 0, "window handle");
      window = reinterpret_cast<HWND>(static_cast<std::uintptr_t>(value));
      continue;
    }
    if (argument == L"--duration" && index + 1 < argument_count) {
      const auto value = parse_unsigned(arguments[++index], 10, "duration");
      if (value == 0 || value > 3'600) {
        throw std::invalid_argument(
            "Duration must be between 1 and 3600 seconds");
      }
      duration_seconds = static_cast<std::uint32_t>(value);
      continue;
    }
    if (argument == L"--fps" && index + 1 < argument_count) {
      const auto value = parse_unsigned(arguments[++index], 10, "frame rate");
      if (value == 0 || value > 240) {
        throw std::invalid_argument("Frame rate must be between 1 and 240");
      }
      frames_per_second = static_cast<std::uint32_t>(value);
      continue;
    }
    if (argument == L"--video-only") {
      video_only = true;
      continue;
    }
    if (argument == L"--preview") {
      show_preview = true;
      continue;
    }
    throw std::invalid_argument("Unknown or incomplete capture argument");
  }

  if (window == nullptr) {
    throw std::invalid_argument("Capture requires --window <hwnd>");
  }

  DWORD process_id = 0;
  GetWindowThreadProcessId(window, &process_id);
  std::stop_source audio_stop_source;
  auto live_status = std::make_shared<chatto::capture::LiveCaptureStatus>();
  std::future<chatto::capture::AudioCaptureMetrics> audio_future;
  if (!video_only && process_id != 0) {
    audio_future = std::async(
        std::launch::async, [process_id, duration_seconds, live_status,
                             stop_token = audio_stop_source.get_token()] {
          winrt::init_apartment(winrt::apartment_type::multi_threaded);
          return chatto::capture::capture_process_audio(
              process_id, std::chrono::seconds(duration_seconds), stop_token,
              live_status);
        });
  }

  chatto::capture::VideoCaptureMetrics metrics;
  try {
    metrics = chatto::capture::capture_window_video(
        window, std::chrono::seconds(duration_seconds), frames_per_second,
        show_preview, live_status);
  } catch (...) {
    audio_stop_source.request_stop();
    if (audio_future.valid()) {
      try {
        static_cast<void>(audio_future.get());
      } catch (...) {
        // Preserve the video failure that initiated cancellation.
      }
    }
    throw;
  }
  audio_stop_source.request_stop();
  std::wcout << std::fixed << std::setprecision(2) << L"video_frames="
             << metrics.frames << L"\n"
             << L"video_size=" << metrics.width << L"x" << metrics.height
             << L"\n"
             << L"video_timestamp_span_seconds="
             << metrics.timestamp_span_seconds << L"\n"
             << L"video_observed_fps=" << metrics.observed_frames_per_second
             << L"\n"
             << L"video_longest_interval_ms="
             << metrics.longest_frame_interval_ms << L"\n"
             << L"video_inferred_gaps=" << metrics.inferred_gaps << L"\n"
             << L"video_resizes=" << metrics.resizes << L"\n"
             << L"video_sampled_frames=" << metrics.sampled_frames << L"\n"
             << L"video_changed_samples=" << metrics.changed_samples << L"\n"
             << L"video_black_samples=" << metrics.black_samples << L"\n"
             << L"video_sampled_luminance_min_mean_max="
             << static_cast<std::uint32_t>(metrics.sampled_luminance_min)
             << L"/" << metrics.sampled_luminance_mean << L"/"
             << static_cast<std::uint32_t>(metrics.sampled_luminance_max)
             << L"\n"
             << L"probe_wall_duration_seconds=" << metrics.wall_duration_seconds
             << L"\n"
             << L"probe_cpu_seconds=" << metrics.process_cpu_seconds << L"\n"
             << L"probe_cpu_single_core_percent="
             << metrics.process_cpu_single_core_percent << L"\n"
             << L"probe_peak_working_set_bytes="
             << metrics.peak_working_set_bytes << L"\n"
             << L"source_closed="
             << (metrics.source_closed ? L"true" : L"false") << L"\n";
  if (!metrics.error.empty()) {
    std::wcout << L"video_error=" << std::quoted(metrics.error) << L"\n";
    return 2;
  }

  if (audio_future.valid()) {
    const auto audio_metrics = audio_future.get();
    print_audio_metrics(audio_metrics);
    if (metrics.first_timestamp_100ns != 0 &&
        audio_metrics.first_timestamp_100ns != 0) {
      const auto start_delta =
          static_cast<std::int64_t>(audio_metrics.first_timestamp_100ns) -
          metrics.first_timestamp_100ns;
      std::wcout << std::fixed << std::setprecision(2) << L"av_start_delta_ms="
                 << static_cast<double>(start_delta) / 10'000.0 << L"\n";
    }
  }
  return metrics.frames > 0 ? 0 : 2;
}

int capture_audio(const int argument_count, wchar_t *arguments[]) {
  DWORD process_id = 0;
  std::uint32_t duration_seconds = 15;
  for (int index = 2; index < argument_count; ++index) {
    const std::wstring_view argument(arguments[index]);
    if (argument == L"--process" && index + 1 < argument_count) {
      const auto value =
          parse_unsigned(arguments[++index], 10, "process identifier");
      if (value == 0 || value > std::numeric_limits<DWORD>::max()) {
        throw std::invalid_argument("Process identifier is out of range");
      }
      process_id = static_cast<DWORD>(value);
      continue;
    }
    if (argument == L"--duration" && index + 1 < argument_count) {
      const auto value = parse_unsigned(arguments[++index], 10, "duration");
      if (value == 0 || value > 3'600) {
        throw std::invalid_argument(
            "Duration must be between 1 and 3600 seconds");
      }
      duration_seconds = static_cast<std::uint32_t>(value);
      continue;
    }
    throw std::invalid_argument("Unknown or incomplete audio argument");
  }
  if (process_id == 0) {
    throw std::invalid_argument("Audio capture requires --process <pid>");
  }

  const auto metrics = chatto::capture::capture_process_audio(
      process_id, std::chrono::seconds(duration_seconds));
  print_audio_metrics(metrics);
  return metrics.packets > 0 ? 0 : 2;
}

chatto::capture::PublisherCredential read_publisher_credential() {
  std::string input;
  std::getline(std::cin, input);
  if (input.empty() || input.size() > 96 * 1024) {
    throw std::invalid_argument("The publisher credential is invalid");
  }
  const auto value =
      winrt::Windows::Data::Json::JsonObject::Parse(winrt::to_hstring(input));
  if (value.GetNamedNumber(L"protocolVersion", 0) != 1) {
    throw std::invalid_argument(
        "The publisher credential protocol is unsupported");
  }
  return {
      .livekit_url = winrt::to_string(value.GetNamedString(L"livekitURL")),
      .token = winrt::to_string(value.GetNamedString(L"token")),
      .e2ee_key = winrt::to_string(value.GetNamedString(L"e2eeKey")),
  };
}

void monitor_publisher_control(std::stop_source stop_source,
                               const std::atomic<bool> &finished) {
  const HANDLE input = GetStdHandle(STD_INPUT_HANDLE);
  if (input == nullptr || input == INVALID_HANDLE_VALUE) {
    return;
  }

  std::string pending;
  while (!finished.load(std::memory_order_relaxed) &&
         !stop_source.stop_requested()) {
    DWORD available = 0;
    if (!PeekNamedPipe(input, nullptr, 0, nullptr, &available, nullptr)) {
      if (GetLastError() == ERROR_BROKEN_PIPE) {
        stop_source.request_stop();
      }
      return;
    }
    if (available == 0) {
      std::this_thread::sleep_for(std::chrono::milliseconds(50));
      continue;
    }

    std::array<char, 256> buffer{};
    DWORD read = 0;
    constexpr DWORD buffer_size = static_cast<DWORD>(buffer.size());
    if (!ReadFile(input, buffer.data(), std::min(available, buffer_size), &read,
                  nullptr)) {
      if (GetLastError() == ERROR_BROKEN_PIPE) {
        stop_source.request_stop();
      }
      return;
    }
    pending.append(buffer.data(), read);
    const auto line_end = pending.find('\n');
    if (line_end == std::string::npos) {
      if (pending.size() > 256) {
        stop_source.request_stop();
        return;
      }
      continue;
    }
    if (pending.substr(0, line_end) == "stop" ||
        pending.substr(0, line_end) == "stop\r") {
      stop_source.request_stop();
    }
    return;
  }
}

int publish(const int argument_count, wchar_t *arguments[]) {
  HWND window = nullptr;
  HMONITOR monitor = nullptr;
  std::wstring expected_application_identifier;
  std::uint32_t frames_per_second = 60;
  std::uint32_t maximum_width = 1920;
  std::uint32_t maximum_height = 1080;
  for (int index = 2; index < argument_count; ++index) {
    const std::wstring_view argument(arguments[index]);
    if (argument == L"--window" && index + 1 < argument_count) {
      const auto value = parse_unsigned(arguments[++index], 0, "window handle");
      window = reinterpret_cast<HWND>(static_cast<std::uintptr_t>(value));
      continue;
    }
    if (argument == L"--display" && index + 1 < argument_count) {
      const auto value =
          parse_unsigned(arguments[++index], 0, "monitor handle");
      monitor = reinterpret_cast<HMONITOR>(static_cast<std::uintptr_t>(value));
      continue;
    }
    if (argument == L"--expected-window-bundle" && index + 1 < argument_count) {
      expected_application_identifier = arguments[++index];
      continue;
    }
    if (argument == L"--fps" && index + 1 < argument_count) {
      const auto value = parse_unsigned(arguments[++index], 10, "frame rate");
      if (value == 0 || value > 60) {
        throw std::invalid_argument("Frame rate must be between 1 and 60");
      }
      frames_per_second = static_cast<std::uint32_t>(value);
      continue;
    }
    if (argument == L"--max-width" && index + 1 < argument_count) {
      const auto value =
          parse_unsigned(arguments[++index], 10, "maximum width");
      if (value == 0 || value > 16'384) {
        throw std::invalid_argument("Maximum width is out of range");
      }
      maximum_width = static_cast<std::uint32_t>(value);
      continue;
    }
    if (argument == L"--max-height" && index + 1 < argument_count) {
      const auto value =
          parse_unsigned(arguments[++index], 10, "maximum height");
      if (value == 0 || value > 16'384) {
        throw std::invalid_argument("Maximum height is out of range");
      }
      maximum_height = static_cast<std::uint32_t>(value);
      continue;
    }
    throw std::invalid_argument("Unknown or incomplete publish argument");
  }
  if ((window == nullptr) == (monitor == nullptr) ||
      (window != nullptr && expected_application_identifier.empty()) ||
      (monitor != nullptr && !expected_application_identifier.empty())) {
    throw std::invalid_argument(
        "Publishing requires either a display or a window and its expected "
        "application identity");
  }
  const auto credential = read_publisher_credential();
  auto preview_pipe = std::make_shared<EncodedPreviewPipe>();
  chatto::capture::EncodedPreviewCallback preview_callback =
      [preview_pipe](const std::span<const std::uint8_t> data,
                     const std::int64_t timestamp_us, const bool key_frame) {
        preview_pipe->write(data, timestamp_us, key_frame);
      };
  std::stop_source stop_source;
  std::atomic<bool> control_finished = false;
  std::thread control_thread(monitor_publisher_control, stop_source,
                             std::cref(control_finished));
  try {
    const int result = window != nullptr
                           ? chatto::capture::publish_window(
                                 window, expected_application_identifier,
                                 frames_per_second, maximum_width,
                                 maximum_height, credential, preview_callback,
                                 stop_source.get_token())
                           : chatto::capture::publish_display(
                                 monitor, frames_per_second, maximum_width,
                                 maximum_height, credential, preview_callback,
                                 stop_source.get_token());
    control_finished.store(true, std::memory_order_relaxed);
    control_thread.join();
    return result;
  } catch (...) {
    control_finished.store(true, std::memory_order_relaxed);
    control_thread.join();
    throw;
  }
}

} // namespace

int wmain(const int argument_count, wchar_t *arguments[]) {
  try {
    winrt::init_apartment(winrt::apartment_type::multi_threaded);

    if (argument_count == 2 && std::wstring_view(arguments[1]) == L"support") {
      return print_support();
    }

    if (argument_count >= 2 && std::wstring_view(arguments[1]) == L"list") {
      bool include_titles = false;
      for (int index = 2; index < argument_count; ++index) {
        if (std::wstring_view(arguments[index]) == L"--include-titles") {
          include_titles = true;
          continue;
        }
        std::wcerr << L"Unknown argument: " << arguments[index] << L"\n";
        print_usage();
        return 1;
      }
      return list_windows(include_titles);
    }

    if (argument_count >= 2 &&
        std::wstring_view(arguments[1]) == L"list-json") {
      return list_windows_json(argument_count, arguments);
    }

    if (argument_count >= 2 && std::wstring_view(arguments[1]) == L"capture") {
      return capture_window(argument_count, arguments);
    }

    if (argument_count >= 2 && std::wstring_view(arguments[1]) == L"audio") {
      return capture_audio(argument_count, arguments);
    }

    if (argument_count >= 2 && std::wstring_view(arguments[1]) == L"publish") {
      return publish(argument_count, arguments);
    }

    print_usage();
    return 1;
  } catch (const winrt::hresult_error &error) {
    std::wcerr << L"Windows capture probe failed: " << error.message().c_str()
               << L" (0x" << std::hex
               << static_cast<std::uint32_t>(error.code()) << L")\n";
    return 1;
  } catch (const std::exception &error) {
    std::cerr << "Windows capture probe failed: " << error.what() << "\n";
    return 1;
  }
}
