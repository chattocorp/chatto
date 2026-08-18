// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#pragma once

#include <cstdint>
#include <optional>
#include <string>
#include <vector>

#include <windows.h>

namespace chatto::capture {

struct WindowSource {
  HWND handle;
  std::uint32_t process_id;
  std::wstring application_name;
  std::wstring application_identifier;
  std::wstring title;
  std::uint32_t width;
  std::uint32_t height;
};

struct DisplaySource {
  HMONITOR handle;
  std::uint32_t display_index;
  bool is_main_display;
  std::uint32_t width;
  std::uint32_t height;
};

[[nodiscard]] bool is_candidate_window(bool visible, bool cloaked, bool owned,
                                       std::uint32_t width,
                                       std::uint32_t height);

[[nodiscard]] std::vector<WindowSource> enumerate_window_sources();

/** Enumerate active physical and virtual desktop monitors for display capture. */
[[nodiscard]] std::vector<DisplaySource> enumerate_display_sources();

/** Whether a temporary monitor handle still identifies an active display. */
[[nodiscard]] bool is_display_capture_candidate(HMONITOR monitor);

/** Whether an existing HWND still satisfies the window-capture source policy.
 */
[[nodiscard]] bool is_window_capture_candidate(HWND window);

[[nodiscard]] bool
window_matches_application(HWND window,
                           const std::wstring &expected_identifier);

/** Choose the best replacement capture window without reusing a stale handle.
 */
[[nodiscard]] std::optional<WindowSource>
select_replacement_window_source(const std::vector<WindowSource> &sources,
                                 const std::wstring &expected_identifier,
                                 std::uint32_t preferred_process_id,
                                 HWND stale_window);

} // namespace chatto::capture
