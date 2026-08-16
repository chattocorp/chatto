// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#include "window_sources.h"

#include <algorithm>
#include <array>
#include <filesystem>
#include <iomanip>
#include <sstream>
#include <string>
#include <system_error>

#include <bcrypt.h>
#include <dwmapi.h>

namespace chatto::capture {
namespace {

constexpr std::uint32_t kMinimumWidth = 320;
constexpr std::uint32_t kMinimumHeight = 180;

[[nodiscard]] std::wstring window_title(HWND window) {
  const int length = GetWindowTextLengthW(window);
  if (length <= 0) {
    return {};
  }

  std::wstring title(static_cast<std::size_t>(length) + 1, L'\0');
  const int copied = GetWindowTextW(window, title.data(), length + 1);
  if (copied <= 0) {
    return {};
  }
  title.resize(static_cast<std::size_t>(copied));
  return title;
}

[[nodiscard]] std::wstring process_image_path(std::uint32_t process_id) {
  const HANDLE process =
      OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, process_id);
  if (process == nullptr) {
    return {};
  }

  std::wstring path(32'768, L'\0');
  DWORD size = static_cast<DWORD>(path.size());
  const BOOL queried =
      QueryFullProcessImageNameW(process, 0, path.data(), &size);
  CloseHandle(process);
  if (!queried || size == 0) {
    return {};
  }

  path.resize(size);
  return path;
}

[[nodiscard]] std::wstring application_identifier(const std::wstring &path) {
  if (path.empty()) {
    return {};
  }
  std::wstring normalized = path;
  std::transform(normalized.begin(), normalized.end(), normalized.begin(),
                 towlower);
  std::array<UCHAR, 32> digest{};
  BCRYPT_ALG_HANDLE algorithm = nullptr;
  NTSTATUS status = BCryptOpenAlgorithmProvider(
      &algorithm, BCRYPT_SHA256_ALGORITHM, nullptr, 0);
  if (status < 0) {
    return {};
  }
  status = BCryptHash(algorithm, nullptr, 0,
                      reinterpret_cast<PUCHAR>(normalized.data()),
                      static_cast<ULONG>(normalized.size() * sizeof(wchar_t)),
                      digest.data(), static_cast<ULONG>(digest.size()));
  BCryptCloseAlgorithmProvider(algorithm, 0);
  if (status < 0) {
    return {};
  }
  std::wostringstream encoded;
  encoded << L"windows-sha256:" << std::hex << std::setfill(L'0');
  for (const auto byte : digest) {
    encoded << std::setw(2) << static_cast<unsigned int>(byte);
  }
  return encoded.str();
}

[[nodiscard]] bool is_cloaked(HWND window) {
  DWORD cloaked = 0;
  return SUCCEEDED(DwmGetWindowAttribute(window, DWMWA_CLOAKED, &cloaked,
                                         sizeof(cloaked))) &&
         cloaked != 0;
}

[[nodiscard]] RECT frame_bounds(HWND window) {
  RECT bounds{};
  if (SUCCEEDED(DwmGetWindowAttribute(window, DWMWA_EXTENDED_FRAME_BOUNDS,
                                      &bounds, sizeof(bounds)))) {
    return bounds;
  }

  GetWindowRect(window, &bounds);
  return bounds;
}

BOOL CALLBACK collect_window(HWND window, LPARAM parameter) {
  auto &sources = *reinterpret_cast<std::vector<WindowSource> *>(parameter);
  const RECT bounds = frame_bounds(window);
  const auto width =
      static_cast<std::uint32_t>(std::max(0L, bounds.right - bounds.left));
  const auto height =
      static_cast<std::uint32_t>(std::max(0L, bounds.bottom - bounds.top));

  if (!is_candidate_window(IsWindowVisible(window) != FALSE, is_cloaked(window),
                           GetWindow(window, GW_OWNER) != nullptr, width,
                           height)) {
    return TRUE;
  }

  std::uint32_t process_id = 0;
  GetWindowThreadProcessId(window, reinterpret_cast<DWORD *>(&process_id));
  const std::wstring title = window_title(window);
  const std::wstring image_path = process_image_path(process_id);
  const std::wstring identifier = application_identifier(image_path);
  if (process_id == 0 || title.empty() || image_path.empty() ||
      identifier.empty()) {
    return TRUE;
  }

  sources.push_back(WindowSource{
      .handle = window,
      .process_id = process_id,
      .application_name =
          std::filesystem::path(image_path).filename().wstring(),
      .application_identifier = identifier,
      .title = title,
      .width = width,
      .height = height,
  });
  return TRUE;
}

BOOL CALLBACK collect_display(HMONITOR monitor, HDC, RECT *bounds,
                              LPARAM parameter) {
  auto &sources = *reinterpret_cast<std::vector<DisplaySource> *>(parameter);
  MONITORINFO information{};
  information.cbSize = sizeof(information);
  if (!GetMonitorInfoW(monitor, &information)) {
    return TRUE;
  }
  const auto width = static_cast<std::uint32_t>(
      std::max(0L, bounds->right - bounds->left));
  const auto height = static_cast<std::uint32_t>(
      std::max(0L, bounds->bottom - bounds->top));
  if (width == 0 || height == 0) {
    return TRUE;
  }
  sources.push_back({
      .handle = monitor,
      .display_index = 0,
      .is_main_display =
          (information.dwFlags & MONITORINFOF_PRIMARY) != 0,
      .width = width,
      .height = height,
  });
  return TRUE;
}

} // namespace

bool is_candidate_window(const bool visible, const bool cloaked,
                         const bool owned, const std::uint32_t width,
                         const std::uint32_t height) {
  return visible && !cloaked && !owned && width >= kMinimumWidth &&
         height >= kMinimumHeight;
}

std::vector<WindowSource> enumerate_window_sources() {
  std::vector<WindowSource> sources;
  EnumWindows(collect_window, reinterpret_cast<LPARAM>(&sources));
  return sources;
}

std::vector<DisplaySource> enumerate_display_sources() {
  std::vector<DisplaySource> sources;
  EnumDisplayMonitors(nullptr, nullptr, collect_display,
                      reinterpret_cast<LPARAM>(&sources));
  std::stable_sort(sources.begin(), sources.end(),
                   [](const DisplaySource &left, const DisplaySource &right) {
                     return left.is_main_display && !right.is_main_display;
                   });
  for (std::size_t index = 0; index < sources.size(); ++index) {
    sources[index].display_index = static_cast<std::uint32_t>(index + 1);
  }
  return sources;
}

bool is_display_capture_candidate(HMONITOR monitor) {
  if (!monitor) {
    return false;
  }
  MONITORINFO information{};
  information.cbSize = sizeof(information);
  return GetMonitorInfoW(monitor, &information) != FALSE;
}

bool is_window_capture_candidate(HWND window) {
  if (!IsWindow(window)) {
    return false;
  }
  const RECT bounds = frame_bounds(window);
  const auto width =
      static_cast<std::uint32_t>(std::max(0L, bounds.right - bounds.left));
  const auto height =
      static_cast<std::uint32_t>(std::max(0L, bounds.bottom - bounds.top));
  return is_candidate_window(
      IsWindowVisible(window) != FALSE, is_cloaked(window),
      GetWindow(window, GW_OWNER) != nullptr, width, height);
}

bool window_matches_application(HWND window,
                                const std::wstring &expected_identifier) {
  if (!IsWindow(window) || expected_identifier.empty()) {
    return false;
  }
  DWORD process_id = 0;
  GetWindowThreadProcessId(window, &process_id);
  return process_id != 0 &&
         application_identifier(process_image_path(process_id)) ==
             expected_identifier;
}

std::optional<WindowSource>
select_replacement_window_source(const std::vector<WindowSource> &sources,
                                 const std::wstring &expected_identifier,
                                 const std::uint32_t preferred_process_id,
                                 HWND stale_window) {
  const WindowSource *selected = nullptr;
  for (const auto &source : sources) {
    if (source.handle == stale_window ||
        source.application_identifier != expected_identifier) {
      continue;
    }
    if (selected == nullptr ||
        (source.process_id == preferred_process_id &&
         selected->process_id != preferred_process_id) ||
        (source.process_id == selected->process_id &&
         static_cast<std::uint64_t>(source.width) * source.height >
             static_cast<std::uint64_t>(selected->width) * selected->height)) {
      selected = &source;
    }
  }
  if (selected == nullptr) {
    return std::nullopt;
  }
  return *selected;
}

} // namespace chatto::capture
