// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#include "video_capture.h"

#include "live_status.h"
#include "preview_window.h"

#include <algorithm>
#include <condition_variable>
#include <cstring>
#include <limits>
#include <memory>
#include <mutex>
#include <optional>
#include <thread>

#include <d3d11.h>
#include <d3d11_4.h>
#include <dxgi1_2.h>
#include <psapi.h>
#include <windows.graphics.capture.interop.h>
#include <windows.graphics.directx.direct3d11.interop.h>
#include <winrt/Windows.Foundation.h>
#include <winrt/Windows.Graphics.Capture.h>
#include <winrt/Windows.Graphics.DirectX.Direct3D11.h>
#include <winrt/Windows.Graphics.DirectX.h>
#include <winrt/base.h>

namespace chatto::capture {
namespace {

using winrt::Windows::Graphics::Capture::Direct3D11CaptureFramePool;
using winrt::Windows::Graphics::Capture::GraphicsCaptureItem;
using winrt::Windows::Graphics::Capture::GraphicsCaptureSession;
using winrt::Windows::Graphics::DirectX::DirectXPixelFormat;
using winrt::Windows::Graphics::DirectX::Direct3D11::IDirect3DDevice;

struct CaptureState {
  std::mutex mutex;
  std::condition_variable changed;
  std::condition_variable callbacks_changed;
  VideoCaptureMetrics metrics;
  std::optional<std::int64_t> first_timestamp;
  std::optional<std::int64_t> previous_timestamp;
  std::int64_t last_timestamp = 0;
  std::int64_t inferred_gap_threshold = 0;
  std::uint32_t pool_width = 0;
  std::uint32_t pool_height = 0;
  double sampled_luminance_total = 0;
  std::optional<std::uint64_t> previous_sample_hash;
  std::chrono::steady_clock::time_point last_frame_at =
      std::chrono::steady_clock::now();
  std::size_t callbacks_in_flight = 0;
  bool accepting_callbacks = true;
  bool finished = false;
};

class CaptureCallbackLease {
public:
  explicit CaptureCallbackLease(std::shared_ptr<CaptureState> state)
      : state_(std::move(state)) {
    std::scoped_lock lock(state_->mutex);
    if (state_->accepting_callbacks) {
      state_->callbacks_in_flight += 1;
      acquired_ = true;
    }
  }

  CaptureCallbackLease(const CaptureCallbackLease &) = delete;
  CaptureCallbackLease &operator=(const CaptureCallbackLease &) = delete;

  ~CaptureCallbackLease() {
    if (!acquired_) {
      return;
    }
    {
      std::scoped_lock lock(state_->mutex);
      state_->callbacks_in_flight -= 1;
    }
    state_->callbacks_changed.notify_all();
  }

  [[nodiscard]] explicit operator bool() const { return acquired_; }

private:
  std::shared_ptr<CaptureState> state_;
  bool acquired_ = false;
};

void stop_and_wait_for_capture_callbacks(
    const std::shared_ptr<CaptureState> &state) {
  std::unique_lock lock(state->mutex);
  state->accepting_callbacks = false;
  state->callbacks_changed.wait(
      lock, [&state] { return state->callbacks_in_flight == 0; });
}

struct FrameSample {
  std::uint64_t hash = 0;
  double mean = 0;
  std::uint8_t minimum = 0;
  std::uint8_t maximum = 0;
};

[[nodiscard]] std::uint64_t file_time_ticks(const FILETIME &time) {
  ULARGE_INTEGER value{};
  value.LowPart = time.dwLowDateTime;
  value.HighPart = time.dwHighDateTime;
  return value.QuadPart;
}

[[nodiscard]] std::uint64_t process_cpu_ticks() {
  FILETIME creation{};
  FILETIME exit{};
  FILETIME kernel{};
  FILETIME user{};
  winrt::check_bool(
      GetProcessTimes(GetCurrentProcess(), &creation, &exit, &kernel, &user));
  return file_time_ticks(kernel) + file_time_ticks(user);
}

void record_process_metrics(
    VideoCaptureMetrics &metrics,
    const std::chrono::steady_clock::duration wall_duration,
    const std::uint64_t starting_cpu_ticks) {
  metrics.wall_duration_seconds =
      std::chrono::duration<double>(wall_duration).count();
  metrics.process_cpu_seconds =
      static_cast<double>(process_cpu_ticks() - starting_cpu_ticks) /
      10'000'000.0;
  if (metrics.wall_duration_seconds > 0) {
    metrics.process_cpu_single_core_percent =
        metrics.process_cpu_seconds / metrics.wall_duration_seconds * 100.0;
  }

  PROCESS_MEMORY_COUNTERS_EX memory{};
  memory.cb = sizeof(memory);
  winrt::check_bool(GetProcessMemoryInfo(
      GetCurrentProcess(), reinterpret_cast<PROCESS_MEMORY_COUNTERS *>(&memory),
      sizeof(memory)));
  metrics.peak_working_set_bytes = memory.PeakWorkingSetSize;
}

[[nodiscard]] GraphicsCaptureItem create_capture_item(HWND window) {
  auto interop = winrt::get_activation_factory<GraphicsCaptureItem,
                                               IGraphicsCaptureItemInterop>();
  GraphicsCaptureItem item{nullptr};
  winrt::check_hresult(interop->CreateForWindow(
      window, winrt::guid_of<GraphicsCaptureItem>(), winrt::put_abi(item)));
  return item;
}

[[nodiscard]] GraphicsCaptureItem create_capture_item(HMONITOR monitor) {
  auto interop = winrt::get_activation_factory<GraphicsCaptureItem,
                                               IGraphicsCaptureItemInterop>();
  GraphicsCaptureItem item{nullptr};
  winrt::check_hresult(interop->CreateForMonitor(
      monitor, winrt::guid_of<GraphicsCaptureItem>(), winrt::put_abi(item)));
  return item;
}

struct DeviceResources {
  winrt::com_ptr<ID3D11Device> d3d_device;
  winrt::com_ptr<ID3D11DeviceContext> d3d_context;
  IDirect3DDevice winrt_device{nullptr};
};

struct DuplicationResources {
  winrt::com_ptr<ID3D11Device> d3d_device;
  winrt::com_ptr<ID3D11DeviceContext> d3d_context;
  winrt::com_ptr<IDXGIOutputDuplication> duplication;
  DXGI_OUTPUT_DESC output_description{};
};

[[nodiscard]] DeviceResources create_device() {
  DeviceResources resources;
  constexpr D3D_FEATURE_LEVEL feature_levels[] = {
      D3D_FEATURE_LEVEL_11_1,
      D3D_FEATURE_LEVEL_11_0,
  };
  D3D_FEATURE_LEVEL selected_feature_level{};

  winrt::check_hresult(
      D3D11CreateDevice(nullptr, D3D_DRIVER_TYPE_HARDWARE, nullptr,
                        D3D11_CREATE_DEVICE_BGRA_SUPPORT, feature_levels,
                        static_cast<UINT>(std::size(feature_levels)),
                        D3D11_SDK_VERSION, resources.d3d_device.put(),
                        &selected_feature_level, resources.d3d_context.put()));

  const auto multithread = resources.d3d_context.as<ID3D11Multithread>();
  multithread->SetMultithreadProtected(TRUE);

  const auto dxgi_device = resources.d3d_device.as<IDXGIDevice>();
  winrt::com_ptr<IInspectable> inspectable;
  winrt::check_hresult(CreateDirect3D11DeviceFromDXGIDevice(dxgi_device.get(),
                                                            inspectable.put()));
  resources.winrt_device = inspectable.as<IDirect3DDevice>();
  return resources;
}

[[nodiscard]] DuplicationResources
create_duplication_resources(const HMONITOR monitor) {
  winrt::com_ptr<IDXGIFactory1> factory;
  winrt::check_hresult(
      CreateDXGIFactory1(__uuidof(IDXGIFactory1), factory.put_void()));

  for (UINT adapter_index = 0;; ++adapter_index) {
    winrt::com_ptr<IDXGIAdapter1> adapter;
    const HRESULT adapter_result =
        factory->EnumAdapters1(adapter_index, adapter.put());
    if (adapter_result == DXGI_ERROR_NOT_FOUND) {
      break;
    }
    winrt::check_hresult(adapter_result);

    for (UINT output_index = 0;; ++output_index) {
      winrt::com_ptr<IDXGIOutput> output;
      const HRESULT output_result =
          adapter->EnumOutputs(output_index, output.put());
      if (output_result == DXGI_ERROR_NOT_FOUND) {
        break;
      }
      winrt::check_hresult(output_result);

      DXGI_OUTPUT_DESC output_description{};
      winrt::check_hresult(output->GetDesc(&output_description));
      if (output_description.Monitor != monitor) {
        continue;
      }

      DuplicationResources resources;
      constexpr D3D_FEATURE_LEVEL feature_levels[] = {
          D3D_FEATURE_LEVEL_11_1,
          D3D_FEATURE_LEVEL_11_0,
      };
      D3D_FEATURE_LEVEL selected_feature_level{};
      winrt::check_hresult(D3D11CreateDevice(
          adapter.get(), D3D_DRIVER_TYPE_UNKNOWN, nullptr,
          D3D11_CREATE_DEVICE_BGRA_SUPPORT, feature_levels,
          static_cast<UINT>(std::size(feature_levels)), D3D11_SDK_VERSION,
          resources.d3d_device.put(), &selected_feature_level,
          resources.d3d_context.put()));
      const auto multithread = resources.d3d_context.as<ID3D11Multithread>();
      multithread->SetMultithreadProtected(TRUE);
      const auto output1 = output.as<IDXGIOutput1>();
      winrt::check_hresult(output1->DuplicateOutput(
          resources.d3d_device.get(), resources.duplication.put()));
      resources.output_description = output_description;
      return resources;
    }
  }
  throw std::runtime_error(
      "The fullscreen window's monitor cannot be duplicated");
}

[[nodiscard]] FrameSample
sample_texture(ID3D11Device &device, ID3D11DeviceContext &context,
               ID3D11Texture2D &texture,
               const D3D11_TEXTURE2D_DESC &description,
               winrt::com_ptr<ID3D11Texture2D> &staging_texture) {
  D3D11_TEXTURE2D_DESC staging_description{};
  if (staging_texture) {
    staging_texture->GetDesc(&staging_description);
  }
  if (!staging_texture || staging_description.Width != description.Width ||
      staging_description.Height != description.Height ||
      staging_description.Format != description.Format) {
    staging_description = description;
    staging_description.BindFlags = 0;
    staging_description.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
    staging_description.MiscFlags = 0;
    staging_description.Usage = D3D11_USAGE_STAGING;
    staging_texture = nullptr;
    winrt::check_hresult(device.CreateTexture2D(&staging_description, nullptr,
                                                staging_texture.put()));
  }

  context.CopyResource(staging_texture.get(), &texture);
  D3D11_MAPPED_SUBRESOURCE mapped{};
  winrt::check_hresult(
      context.Map(staging_texture.get(), 0, D3D11_MAP_READ, 0, &mapped));

  FrameSample sample;
  sample.hash = 1'469'598'103'934'665'603ULL;
  sample.minimum = std::numeric_limits<std::uint8_t>::max();
  const std::uint32_t horizontal_step = std::max(1U, description.Width / 64U);
  const std::uint32_t vertical_step = std::max(1U, description.Height / 36U);
  std::uint64_t luminance_total = 0;
  std::uint64_t sample_count = 0;

  for (std::uint32_t y = vertical_step / 2; y < description.Height;
       y += vertical_step) {
    const auto *row = static_cast<const std::uint8_t *>(mapped.pData) +
                      static_cast<std::size_t>(y) * mapped.RowPitch;
    for (std::uint32_t x = horizontal_step / 2; x < description.Width;
         x += horizontal_step) {
      const auto *pixel = row + static_cast<std::size_t>(x) * 4;
      const auto luminance = static_cast<std::uint8_t>(
          (static_cast<std::uint32_t>(pixel[0]) + pixel[1] + pixel[2]) / 3U);
      sample.minimum = std::min(sample.minimum, luminance);
      sample.maximum = std::max(sample.maximum, luminance);
      luminance_total += luminance;
      sample_count += 1;
      sample.hash ^= luminance;
      sample.hash *= 1'099'511'628'211ULL;
    }
  }
  context.Unmap(staging_texture.get(), 0);
  if (sample_count > 0) {
    sample.mean = static_cast<double>(luminance_total) /
                  static_cast<double>(sample_count);
  }
  return sample;
}

[[nodiscard]] VideoFrameData
copy_texture(ID3D11Device &device, ID3D11DeviceContext &context,
             ID3D11Texture2D &texture, const D3D11_TEXTURE2D_DESC &description,
             const std::int64_t timestamp_100ns,
             winrt::com_ptr<ID3D11Texture2D> &staging_texture) {
  const auto readback_start = std::chrono::steady_clock::now();
  D3D11_TEXTURE2D_DESC staging_description{};
  if (staging_texture) {
    staging_texture->GetDesc(&staging_description);
  }
  if (!staging_texture || staging_description.Width != description.Width ||
      staging_description.Height != description.Height ||
      staging_description.Format != description.Format) {
    staging_description = description;
    staging_description.BindFlags = 0;
    staging_description.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
    staging_description.MiscFlags = 0;
    staging_description.Usage = D3D11_USAGE_STAGING;
    staging_texture = nullptr;
    winrt::check_hresult(device.CreateTexture2D(&staging_description, nullptr,
                                                staging_texture.put()));
  }

  context.CopyResource(staging_texture.get(), &texture);
  D3D11_MAPPED_SUBRESOURCE mapped{};
  winrt::check_hresult(
      context.Map(staging_texture.get(), 0, D3D11_MAP_READ, 0, &mapped));
  VideoFrameData result{
      .width = description.Width,
      .height = description.Height,
      .timestamp_100ns = timestamp_100ns,
      .readback_duration_ms = 0,
      .bgra = std::vector<std::uint8_t>(
          static_cast<std::size_t>(description.Width) * description.Height * 4),
  };
  const auto row_bytes = static_cast<std::size_t>(description.Width) * 4;
  for (std::uint32_t row = 0; row < description.Height; ++row) {
    std::memcpy(result.bgra.data() + static_cast<std::size_t>(row) * row_bytes,
                static_cast<const std::uint8_t *>(mapped.pData) +
                    static_cast<std::size_t>(row) * mapped.RowPitch,
                row_bytes);
  }
  context.Unmap(staging_texture.get(), 0);
  result.readback_duration_ms =
      std::chrono::duration<double, std::milli>(
          std::chrono::steady_clock::now() - readback_start)
          .count();
  return result;
}

[[nodiscard]] VideoFrameData
copy_texture_region(ID3D11Device &device, ID3D11DeviceContext &context,
                    ID3D11Texture2D &texture,
                    const D3D11_TEXTURE2D_DESC &description,
                    const D3D11_BOX &region, const std::int64_t timestamp_100ns,
                    winrt::com_ptr<ID3D11Texture2D> &staging_texture) {
  const auto readback_start = std::chrono::steady_clock::now();
  const auto width = region.right - region.left;
  const auto height = region.bottom - region.top;
  D3D11_TEXTURE2D_DESC staging_description{};
  if (staging_texture) {
    staging_texture->GetDesc(&staging_description);
  }
  if (!staging_texture || staging_description.Width != width ||
      staging_description.Height != height ||
      staging_description.Format != description.Format) {
    staging_description = description;
    staging_description.Width = width;
    staging_description.Height = height;
    staging_description.BindFlags = 0;
    staging_description.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
    staging_description.MiscFlags = 0;
    staging_description.Usage = D3D11_USAGE_STAGING;
    staging_texture = nullptr;
    winrt::check_hresult(device.CreateTexture2D(&staging_description, nullptr,
                                                staging_texture.put()));
  }

  context.CopySubresourceRegion(staging_texture.get(), 0, 0, 0, 0, &texture, 0,
                                &region);
  D3D11_MAPPED_SUBRESOURCE mapped{};
  winrt::check_hresult(
      context.Map(staging_texture.get(), 0, D3D11_MAP_READ, 0, &mapped));
  VideoFrameData result{
      .width = width,
      .height = height,
      .timestamp_100ns = timestamp_100ns,
      .readback_duration_ms = 0,
      .bgra = std::vector<std::uint8_t>(static_cast<std::size_t>(width) *
                                        height * 4),
  };
  const auto row_bytes = static_cast<std::size_t>(width) * 4;
  for (std::uint32_t row = 0; row < height; ++row) {
    std::memcpy(result.bgra.data() + static_cast<std::size_t>(row) * row_bytes,
                static_cast<const std::uint8_t *>(mapped.pData) +
                    static_cast<std::size_t>(row) * mapped.RowPitch,
                row_bytes);
  }
  context.Unmap(staging_texture.get(), 0);
  result.readback_duration_ms =
      std::chrono::duration<double, std::milli>(
          std::chrono::steady_clock::now() - readback_start)
          .count();
  return result;
}

[[nodiscard]] std::optional<D3D11_BOX>
window_region_on_output(HWND window, const DXGI_OUTPUT_DESC &output) {
  RECT window_rectangle{};
  if (!GetWindowRect(window, &window_rectangle)) {
    return std::nullopt;
  }
  const auto &desktop = output.DesktopCoordinates;
  const LONG left = std::max(window_rectangle.left, desktop.left);
  const LONG top = std::max(window_rectangle.top, desktop.top);
  const LONG right = std::min(window_rectangle.right, desktop.right);
  const LONG bottom = std::min(window_rectangle.bottom, desktop.bottom);
  if (right <= left || bottom <= top) {
    return std::nullopt;
  }
  return D3D11_BOX{
      .left = static_cast<UINT>(left - desktop.left),
      .top = static_cast<UINT>(top - desktop.top),
      .front = 0,
      .right = static_cast<UINT>(right - desktop.left),
      .bottom = static_cast<UINT>(bottom - desktop.top),
      .back = 1,
  };
}

[[nodiscard]] bool record_frame(
    CaptureState &state,
    const winrt::Windows::Graphics::Capture::Direct3D11CaptureFrame &frame,
    const D3D11_TEXTURE2D_DESC &texture_description,
    const std::optional<FrameSample> &sample) {
  const std::int64_t timestamp = frame.SystemRelativeTime().count();
  const auto content_size = frame.ContentSize();
  std::scoped_lock lock(state.mutex);

  state.metrics.frames += 1;
  state.last_frame_at = std::chrono::steady_clock::now();
  state.metrics.width = texture_description.Width;
  state.metrics.height = texture_description.Height;
  if (!state.first_timestamp) {
    state.first_timestamp = timestamp;
    state.metrics.first_timestamp_100ns = timestamp;
  }
  if (state.previous_timestamp) {
    const std::int64_t interval = timestamp - *state.previous_timestamp;
    if (interval > 0) {
      state.metrics.longest_frame_interval_ms =
          std::max(state.metrics.longest_frame_interval_ms,
                   static_cast<double>(interval) / 10'000.0);
      if (interval > state.inferred_gap_threshold) {
        state.metrics.inferred_gaps += 1;
      }
    }
  }
  state.previous_timestamp = timestamp;
  state.last_timestamp = timestamp;
  state.metrics.last_timestamp_100ns = timestamp;

  if (sample) {
    state.metrics.sampled_frames += 1;
    if (state.previous_sample_hash &&
        *state.previous_sample_hash != sample->hash) {
      state.metrics.changed_samples += 1;
    }
    state.previous_sample_hash = sample->hash;
    if (sample->maximum <= 2) {
      state.metrics.black_samples += 1;
    }
    if (state.metrics.sampled_frames == 1) {
      state.metrics.sampled_luminance_min = sample->minimum;
    } else {
      state.metrics.sampled_luminance_min =
          std::min(state.metrics.sampled_luminance_min, sample->minimum);
    }
    state.metrics.sampled_luminance_max =
        std::max(state.metrics.sampled_luminance_max, sample->maximum);
    state.sampled_luminance_total += sample->mean;
  }

  const auto content_width =
      static_cast<std::uint32_t>(std::max(0, content_size.Width));
  const auto content_height =
      static_cast<std::uint32_t>(std::max(0, content_size.Height));
  if (content_width > 0 && content_height > 0 &&
      (content_width != state.pool_width ||
       content_height != state.pool_height)) {
    state.pool_width = content_width;
    state.pool_height = content_height;
    state.metrics.resizes += 1;
    return true;
  }
  return false;
}

void record_error(CaptureState &state, const winrt::hresult_error &error) {
  {
    std::scoped_lock lock(state.mutex);
    state.metrics.error = error.message().c_str();
    state.finished = true;
  }
  state.changed.notify_all();
}

void record_error(CaptureState &state, const std::exception &error) {
  {
    std::scoped_lock lock(state.mutex);
    state.metrics.error = winrt::to_hstring(error.what()).c_str();
    state.finished = true;
  }
  state.changed.notify_all();
}

void finish_metrics(CaptureState &state) {
  if (state.metrics.sampled_frames > 0) {
    state.metrics.sampled_luminance_mean =
        state.sampled_luminance_total /
        static_cast<double>(state.metrics.sampled_frames);
  }
  if (!state.first_timestamp || state.metrics.frames < 2) {
    return;
  }
  const std::int64_t span = state.last_timestamp - *state.first_timestamp;
  if (span <= 0) {
    return;
  }
  state.metrics.timestamp_span_seconds =
      static_cast<double>(span) / 10'000'000.0;
  state.metrics.observed_frames_per_second =
      static_cast<double>(state.metrics.frames - 1) /
      state.metrics.timestamp_span_seconds;
}

} // namespace

std::pair<std::uint32_t, std::uint32_t> window_capture_size(HWND window) {
  if (!IsWindow(window)) {
    throw std::invalid_argument("The selected window no longer exists");
  }
  const auto size = create_capture_item(window).Size();
  if (size.Width <= 0 || size.Height <= 0) {
    throw std::runtime_error(
        "The selected window has no capturable content size");
  }
  return {
      static_cast<std::uint32_t>(size.Width),
      static_cast<std::uint32_t>(size.Height),
  };
}

bool is_foreground_monitor_covering_window(HWND window) {
  if (!IsWindow(window) || IsIconic(window)) {
    return false;
  }
  const HWND foreground = GetForegroundWindow();
  if (foreground != window && GetAncestor(foreground, GA_ROOT) != window) {
    return false;
  }

  const HMONITOR monitor = MonitorFromWindow(window, MONITOR_DEFAULTTONULL);
  if (!monitor) {
    return false;
  }
  MONITORINFO monitor_information{.cbSize = sizeof(MONITORINFO)};
  RECT window_rectangle{};
  if (!GetMonitorInfoW(monitor, &monitor_information) ||
      !GetWindowRect(window, &window_rectangle)) {
    return false;
  }
  constexpr LONG tolerance = 2;
  const RECT &monitor_rectangle = monitor_information.rcMonitor;
  return window_rectangle.left <= monitor_rectangle.left + tolerance &&
         window_rectangle.top <= monitor_rectangle.top + tolerance &&
         window_rectangle.right >= monitor_rectangle.right - tolerance &&
         window_rectangle.bottom >= monitor_rectangle.bottom - tolerance;
}

VideoCaptureMetrics capture_window_video(
    HWND window, const std::chrono::seconds duration,
    const std::uint32_t requested_frames_per_second, const bool show_preview,
    std::shared_ptr<LiveCaptureStatus> live_status,
    VideoFrameHandler frame_handler, const std::stop_token stop_token,
    const std::chrono::milliseconds frame_stall_timeout,
    const bool switch_on_monitor_covering_presentation) {
  if (!IsWindow(window)) {
    throw std::invalid_argument("The selected window no longer exists");
  }

  const auto wall_start = std::chrono::steady_clock::now();
  const std::uint64_t starting_cpu_ticks = process_cpu_ticks();
  if (duration.count() <= 0 || requested_frames_per_second == 0) {
    throw std::invalid_argument(
        "Capture duration and frame rate must be positive");
  }

  const auto item = create_capture_item(window);
  const auto device = create_device();
  const auto initial_size = item.Size();
  if (initial_size.Width <= 0 || initial_size.Height <= 0) {
    throw std::runtime_error(
        "The selected window has no capturable content size");
  }

  auto state = std::make_shared<CaptureState>();
  state->pool_width = static_cast<std::uint32_t>(initial_size.Width);
  state->pool_height = static_cast<std::uint32_t>(initial_size.Height);
  const auto expected_interval =
      10'000'000LL / static_cast<std::int64_t>(requested_frames_per_second);
  state->inferred_gap_threshold = expected_interval + (expected_interval / 2);
  std::shared_ptr<PreviewWindow> preview;
  if (show_preview) {
    if (!live_status) {
      live_status = std::make_shared<LiveCaptureStatus>();
    }
    preview = std::make_shared<PreviewWindow>(
        *device.d3d_device, *device.d3d_context,
        static_cast<std::uint32_t>(initial_size.Width),
        static_cast<std::uint32_t>(initial_size.Height));
  }

  auto frame_pool = Direct3D11CaptureFramePool::CreateFreeThreaded(
      device.winrt_device, DirectXPixelFormat::B8G8R8A8UIntNormalized, 3,
      initial_size);
  auto session = frame_pool.CreateCaptureSession(item);
  session.IsCursorCaptureEnabled(false);

  const auto frame_token = frame_pool.FrameArrived(
      [state, winrt_device = device.winrt_device,
       d3d_device = device.d3d_device, d3d_context = device.d3d_context,
       preview, frame_handler = std::move(frame_handler),
       staging_texture = winrt::com_ptr<ID3D11Texture2D>{},
       publish_staging_texture = winrt::com_ptr<ID3D11Texture2D>{},
       frames_seen = std::uint64_t{0}](const Direct3D11CaptureFramePool &sender,
                                       const auto &) mutable noexcept {
        CaptureCallbackLease callback(state);
        if (!callback) {
          return;
        }
        try {
          const auto frame = sender.TryGetNextFrame();
          if (frame) {
            const auto surface_access =
                frame.Surface()
                    .as<Windows::Graphics::DirectX::Direct3D11::
                            IDirect3DDxgiInterfaceAccess>();
            winrt::com_ptr<ID3D11Texture2D> texture;
            winrt::check_hresult(surface_access->GetInterface(
                __uuidof(ID3D11Texture2D), texture.put_void()));
            D3D11_TEXTURE2D_DESC texture_description{};
            texture->GetDesc(&texture_description);
            frames_seen += 1;
            std::optional<FrameSample> sample;
            if (frames_seen == 1 || frames_seen % 12 == 0) {
              sample = sample_texture(*d3d_device, *d3d_context, *texture,
                                      texture_description, staging_texture);
            }
            if (preview) {
              preview->present(*texture);
            }
            if (frame_handler) {
              frame_handler(copy_texture(
                  *d3d_device, *d3d_context, *texture, texture_description,
                  frame.SystemRelativeTime().count(), publish_staging_texture));
            }
            const bool resized =
                record_frame(*state, frame, texture_description, sample);
            frame.Close();
            if (resized) {
              winrt::Windows::Graphics::SizeInt32 new_size{};
              {
                std::scoped_lock lock(state->mutex);
                new_size.Width = static_cast<std::int32_t>(state->pool_width);
                new_size.Height = static_cast<std::int32_t>(state->pool_height);
              }
              sender.Recreate(winrt_device,
                              DirectXPixelFormat::B8G8R8A8UIntNormalized, 3,
                              new_size);
            }
          }
        } catch (const winrt::hresult_error &error) {
          record_error(*state, error);
        } catch (const std::exception &error) {
          record_error(*state, error);
        }
      });
  const auto closed_token =
      item.Closed([state](const GraphicsCaptureItem &, const auto &) noexcept {
        {
          std::scoped_lock lock(state->mutex);
          state->metrics.source_closed = true;
          state->finished = true;
        }
        state->changed.notify_all();
      });

  session.StartCapture();
  if (preview) {
    const auto deadline = std::chrono::steady_clock::now() + duration;
    while (std::chrono::steady_clock::now() < deadline && !preview->closed()) {
      preview->pump_messages();
      {
        std::scoped_lock lock(state->mutex);
        if (state->finished) {
          break;
        }
        double live_frames_per_second = 0;
        if (state->first_timestamp && state->metrics.frames >= 2 &&
            state->last_timestamp > *state->first_timestamp) {
          live_frames_per_second =
              static_cast<double>(state->metrics.frames - 1) /
              (static_cast<double>(state->last_timestamp -
                                   *state->first_timestamp) /
               10'000'000.0);
        }
        preview->update_status(
            state->metrics.frames, state->metrics.sampled_frames,
            state->metrics.changed_samples, live_frames_per_second,
            live_status->audio_frames.load(std::memory_order_relaxed),
            live_status->latest_audio_peak.load(std::memory_order_relaxed),
            live_status->audio_discontinuities.load(std::memory_order_relaxed));
      }
      std::this_thread::sleep_for(std::chrono::milliseconds(16));
    }
    std::scoped_lock lock(state->mutex);
    state->finished = true;
  } else {
    std::unique_lock lock(state->mutex);
    const auto deadline = wall_start + duration;
    while (!state->finished && std::chrono::steady_clock::now() < deadline) {
      if (stop_token.stop_requested()) {
        state->metrics.stop_requested = true;
        break;
      }
      if (!IsWindow(window)) {
        state->metrics.source_closed = true;
        break;
      }
      if (switch_on_monitor_covering_presentation &&
          is_foreground_monitor_covering_window(window)) {
        // Window WGC can continue emitting sparse heartbeat frames while a
        // monitor-covering flip-model game bypasses composition. Switch source
        // types based on presentation state rather than waiting for zero
        // frames.
        state->metrics.frame_stalled = true;
        break;
      }
      if (frame_stall_timeout.count() > 0 &&
          std::chrono::steady_clock::now() - state->last_frame_at >=
              frame_stall_timeout) {
        state->metrics.frame_stalled = true;
        break;
      }
      state->changed.wait_for(lock, std::chrono::milliseconds(100),
                              [state] { return state->finished; });
    }
    state->finished = true;
  }

  item.Closed(closed_token);
  frame_pool.FrameArrived(frame_token);
  stop_and_wait_for_capture_callbacks(state);
  session.Close();
  frame_pool.Close();

  std::scoped_lock lock(state->mutex);
  finish_metrics(*state);
  record_process_metrics(state->metrics,
                         std::chrono::steady_clock::now() - wall_start,
                         starting_cpu_ticks);
  return state->metrics;
}

VideoCaptureMetrics capture_monitor_wgc_video_impl(
    HMONITOR monitor, HWND presentation_window,
    const std::chrono::seconds duration,
    const std::uint32_t requested_frames_per_second,
    VideoFrameHandler frame_handler, const std::stop_token stop_token,
    const std::chrono::milliseconds frame_stall_timeout) {
  MONITORINFO monitor_information{};
  monitor_information.cbSize = sizeof(monitor_information);
  if (!monitor || !GetMonitorInfoW(monitor, &monitor_information)) {
    throw std::invalid_argument("The selected monitor no longer exists");
  }
  if (duration.count() <= 0 || requested_frames_per_second == 0) {
    throw std::invalid_argument(
        "Capture duration and frame rate must be positive");
  }
  const auto wall_start = std::chrono::steady_clock::now();
  const std::uint64_t starting_cpu_ticks = process_cpu_ticks();
  const auto item = create_capture_item(monitor);
  const auto device = create_device();
  const auto initial_size = item.Size();
  if (initial_size.Width <= 0 || initial_size.Height <= 0) {
    throw std::runtime_error("The selected monitor has no capturable size");
  }

  auto state = std::make_shared<CaptureState>();
  state->pool_width = static_cast<std::uint32_t>(initial_size.Width);
  state->pool_height = static_cast<std::uint32_t>(initial_size.Height);
  const auto expected_interval =
      10'000'000LL / static_cast<std::int64_t>(requested_frames_per_second);
  state->inferred_gap_threshold = expected_interval + (expected_interval / 2);

  auto frame_pool = Direct3D11CaptureFramePool::CreateFreeThreaded(
      device.winrt_device, DirectXPixelFormat::B8G8R8A8UIntNormalized, 3,
      initial_size);
  auto session = frame_pool.CreateCaptureSession(item);
  session.IsCursorCaptureEnabled(presentation_window == nullptr);

  const auto frame_token = frame_pool.FrameArrived(
      [state, winrt_device = device.winrt_device,
       d3d_device = device.d3d_device, d3d_context = device.d3d_context,
       frame_handler = std::move(frame_handler),
       staging_texture = winrt::com_ptr<ID3D11Texture2D>{}](
          const Direct3D11CaptureFramePool &sender,
          const auto &) mutable noexcept {
        CaptureCallbackLease callback(state);
        if (!callback) {
          return;
        }
        try {
          const auto frame = sender.TryGetNextFrame();
          if (!frame) {
            return;
          }
          const auto surface_access =
              frame.Surface()
                  .as<Windows::Graphics::DirectX::Direct3D11::
                          IDirect3DDxgiInterfaceAccess>();
          winrt::com_ptr<ID3D11Texture2D> texture;
          winrt::check_hresult(surface_access->GetInterface(
              __uuidof(ID3D11Texture2D), texture.put_void()));
          D3D11_TEXTURE2D_DESC texture_description{};
          texture->GetDesc(&texture_description);
          if (frame_handler) {
            frame_handler(copy_texture(
                *d3d_device, *d3d_context, *texture, texture_description,
                frame.SystemRelativeTime().count(), staging_texture));
          }
          const bool resized =
              record_frame(*state, frame, texture_description, std::nullopt);
          frame.Close();
          if (resized) {
            winrt::Windows::Graphics::SizeInt32 new_size{};
            {
              std::scoped_lock lock(state->mutex);
              new_size.Width = static_cast<std::int32_t>(state->pool_width);
              new_size.Height = static_cast<std::int32_t>(state->pool_height);
            }
            sender.Recreate(winrt_device,
                            DirectXPixelFormat::B8G8R8A8UIntNormalized, 3,
                            new_size);
          }
        } catch (const winrt::hresult_error &error) {
          record_error(*state, error);
        } catch (const std::exception &error) {
          record_error(*state, error);
        }
      });
  const auto closed_token =
      item.Closed([state](const GraphicsCaptureItem &, const auto &) noexcept {
        {
          std::scoped_lock lock(state->mutex);
          state->metrics.frame_stalled = true;
          state->finished = true;
        }
        state->changed.notify_all();
      });

  session.StartCapture();
  std::unique_lock lock(state->mutex);
  const auto deadline = wall_start + duration;
  while (!state->finished && std::chrono::steady_clock::now() < deadline) {
    if (stop_token.stop_requested()) {
      state->metrics.stop_requested = true;
      break;
    }
    if (presentation_window && !IsWindow(presentation_window)) {
      state->metrics.source_closed = true;
      break;
    }
    if (presentation_window &&
        (!is_foreground_monitor_covering_window(presentation_window) ||
         MonitorFromWindow(presentation_window, MONITOR_DEFAULTTONULL) !=
             monitor)) {
      state->metrics.presentation_changed = true;
      break;
    }
    if (frame_stall_timeout.count() > 0 &&
        std::chrono::steady_clock::now() - state->last_frame_at >=
            frame_stall_timeout) {
      state->metrics.frame_stalled = true;
      break;
    }
    state->changed.wait_for(lock, std::chrono::milliseconds(100),
                            [state] { return state->finished; });
  }
  state->finished = true;
  lock.unlock();

  item.Closed(closed_token);
  frame_pool.FrameArrived(frame_token);
  stop_and_wait_for_capture_callbacks(state);
  session.Close();
  frame_pool.Close();

  std::scoped_lock final_lock(state->mutex);
  finish_metrics(*state);
  record_process_metrics(state->metrics,
                         std::chrono::steady_clock::now() - wall_start,
                         starting_cpu_ticks);
  return state->metrics;
}

VideoCaptureMetrics capture_monitor_wgc_video(
    HMONITOR monitor, const std::chrono::seconds duration,
    const std::uint32_t requested_frames_per_second,
    VideoFrameHandler frame_handler, const std::stop_token stop_token,
    const std::chrono::milliseconds frame_stall_timeout) {
  return capture_monitor_wgc_video_impl(
      monitor, nullptr, duration, requested_frames_per_second,
      std::move(frame_handler), stop_token, frame_stall_timeout);
}

VideoCaptureMetrics capture_monitor_covering_window_wgc_video(
    HWND window, const std::chrono::seconds duration,
    const std::uint32_t requested_frames_per_second,
    VideoFrameHandler frame_handler, const std::stop_token stop_token,
    const std::chrono::milliseconds frame_stall_timeout) {
  if (!IsWindow(window)) {
    throw std::invalid_argument("The selected window no longer exists");
  }
  const HMONITOR monitor = MonitorFromWindow(window, MONITOR_DEFAULTTONULL);
  if (!monitor) {
    throw std::runtime_error(
        "The monitor-covering window is not attached to a monitor");
  }
  return capture_monitor_wgc_video_impl(
      monitor, window, duration, requested_frames_per_second,
      std::move(frame_handler), stop_token, frame_stall_timeout);
}

VideoCaptureMetrics capture_monitor_covering_window_dxgi_video(
    HWND window, const std::chrono::seconds duration,
    const std::uint32_t requested_frames_per_second,
    VideoFrameHandler frame_handler, const std::stop_token stop_token) {
  if (!IsWindow(window)) {
    throw std::invalid_argument("The selected window no longer exists");
  }
  if (duration.count() <= 0 || requested_frames_per_second == 0) {
    throw std::invalid_argument(
        "Capture duration and frame rate must be positive");
  }

  const auto wall_start = std::chrono::steady_clock::now();
  const std::uint64_t starting_cpu_ticks = process_cpu_ticks();
  VideoCaptureMetrics metrics;
  std::optional<std::int64_t> first_timestamp;
  const auto frame_interval =
      std::chrono::duration_cast<std::chrono::steady_clock::duration>(
          std::chrono::duration<double>(1.0 / requested_frames_per_second));
  auto next_frame_at = wall_start;

  try {
    const HMONITOR monitor = MonitorFromWindow(window, MONITOR_DEFAULTTONULL);
    if (!monitor) {
      throw std::runtime_error(
          "The monitor-covering window is not attached to a monitor");
    }
    auto resources = create_duplication_resources(monitor);
    winrt::com_ptr<ID3D11Texture2D> staging_texture;
    const auto deadline = wall_start + duration;

    while (std::chrono::steady_clock::now() < deadline) {
      if (stop_token.stop_requested()) {
        metrics.stop_requested = true;
        break;
      }
      if (!IsWindow(window)) {
        metrics.source_closed = true;
        break;
      }
      if (!is_foreground_monitor_covering_window(window) ||
          MonitorFromWindow(window, MONITOR_DEFAULTTONULL) != monitor) {
        metrics.presentation_changed = true;
        break;
      }

      DXGI_OUTDUPL_FRAME_INFO frame_information{};
      winrt::com_ptr<IDXGIResource> desktop_resource;
      const HRESULT acquire_result = resources.duplication->AcquireNextFrame(
          100, &frame_information, desktop_resource.put());
      if (acquire_result == DXGI_ERROR_WAIT_TIMEOUT) {
        continue;
      }
      if (acquire_result == DXGI_ERROR_ACCESS_LOST) {
        metrics.frame_stalled = true;
        std::this_thread::sleep_for(std::chrono::milliseconds(100));
        break;
      }
      winrt::check_hresult(acquire_result);

      try {
        const auto now = std::chrono::steady_clock::now();
        if (now >= next_frame_at) {
          const auto region =
              window_region_on_output(window, resources.output_description);
          if (!region) {
            metrics.presentation_changed = true;
          } else {
            const auto texture = desktop_resource.as<ID3D11Texture2D>();
            D3D11_TEXTURE2D_DESC texture_description{};
            texture->GetDesc(&texture_description);
            const std::int64_t timestamp_100ns =
                std::chrono::duration_cast<std::chrono::duration<
                    std::int64_t, std::ratio<1, 10'000'000>>>(
                    now.time_since_epoch())
                    .count();
            auto frame = copy_texture_region(
                *resources.d3d_device, *resources.d3d_context, *texture,
                texture_description, *region, timestamp_100ns, staging_texture);
            metrics.frames += 1;
            metrics.width = frame.width;
            metrics.height = frame.height;
            metrics.last_timestamp_100ns = timestamp_100ns;
            if (!first_timestamp) {
              first_timestamp = timestamp_100ns;
              metrics.first_timestamp_100ns = timestamp_100ns;
            }
            if (frame_handler) {
              frame_handler(std::move(frame));
            }
            next_frame_at = now + frame_interval;
          }
        }
      } catch (...) {
        static_cast<void>(resources.duplication->ReleaseFrame());
        throw;
      }
      winrt::check_hresult(resources.duplication->ReleaseFrame());
      if (metrics.presentation_changed) {
        break;
      }
    }
  } catch (const winrt::hresult_error &error) {
    if (error.code().value ==
        static_cast<std::int32_t>(DXGI_ERROR_ACCESS_LOST)) {
      // Windows invalidates every duplication interface when the producer of
      // the desktop image changes (for example, entering or leaving direct
      // presentation). Return a recoverable stall so the publisher destroys
      // these resources and recreates the interface for the new producer.
      metrics.frame_stalled = true;
      std::this_thread::sleep_for(std::chrono::milliseconds(100));
    } else {
      metrics.error_code = error.code().value;
      metrics.error = error.message().c_str();
    }
  } catch (const std::exception &error) {
    metrics.error = winrt::to_hstring(error.what()).c_str();
  }

  if (first_timestamp && metrics.frames >= 2 &&
      metrics.last_timestamp_100ns > *first_timestamp) {
    const std::int64_t span = metrics.last_timestamp_100ns - *first_timestamp;
    metrics.timestamp_span_seconds = static_cast<double>(span) / 10'000'000.0;
    metrics.observed_frames_per_second =
        static_cast<double>(metrics.frames - 1) /
        metrics.timestamp_span_seconds;
  }
  record_process_metrics(metrics, std::chrono::steady_clock::now() - wall_start,
                         starting_cpu_ticks);
  return metrics;
}

} // namespace chatto::capture
