// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#include <cstdint>
#include <cstdlib>
#include <iostream>

#include "../src/window_sources.h"

namespace {

void expect(const bool condition, const char *message) {
  if (!condition) {
    std::cerr << "FAILED: " << message << "\n";
    std::exit(1);
  }
}

} // namespace

int main() {
  using chatto::capture::is_candidate_window;

  expect(is_candidate_window(true, false, false, 320, 180),
         "minimum visible top-level window is included");
  expect(!is_candidate_window(false, false, false, 1920, 1080),
         "hidden window is excluded");
  expect(!is_candidate_window(true, true, false, 1920, 1080),
         "cloaked window is excluded");
  expect(!is_candidate_window(true, false, true, 1920, 1080),
         "owned window is excluded");
  expect(!is_candidate_window(true, false, false, 319, 1080),
         "narrow implementation window is excluded");
  expect(!is_candidate_window(true, false, false, 1920, 179),
         "short implementation window is excluded");

  const std::wstring expected_identifier = L"windows-sha256:game";
  const auto first_handle = reinterpret_cast<HWND>(std::uintptr_t{1});
  const auto stale_handle = reinterpret_cast<HWND>(std::uintptr_t{2});
  const auto preferred_handle = reinterpret_cast<HWND>(std::uintptr_t{3});
  const std::vector<chatto::capture::WindowSource> sources{
      {.handle = first_handle,
       .process_id = 7,
       .application_identifier = expected_identifier,
       .width = 1920,
       .height = 1080},
      {.handle = stale_handle,
       .process_id = 42,
       .application_identifier = expected_identifier,
       .width = 3840,
       .height = 2160},
      {.handle = preferred_handle,
       .process_id = 42,
       .application_identifier = expected_identifier,
       .width = 1280,
       .height = 720},
  };
  const auto replacement = chatto::capture::select_replacement_window_source(
      sources, expected_identifier, 42, stale_handle);
  expect(replacement && replacement->handle == preferred_handle,
         "replacement prefers the original process and excludes stale handle");

  std::cout << "window source tests passed\n";
  return 0;
}
