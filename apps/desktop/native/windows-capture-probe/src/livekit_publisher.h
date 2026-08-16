// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#pragma once

#include <cstdint>
#include <functional>
#include <span>
#include <stop_token>
#include <string>

#include <windows.h>

namespace chatto::capture {

struct PublisherCredential {
  std::string livekit_url;
  std::string token;
  std::string e2ee_key;
};

using EncodedPreviewCallback =
    std::function<void(std::span<const std::uint8_t>, std::int64_t, bool)>;

[[nodiscard]] int
publish_window(HWND window, const std::wstring &expected_application_identifier,
               std::uint32_t frames_per_second, std::uint32_t maximum_width,
               std::uint32_t maximum_height,
               const PublisherCredential &credential,
               EncodedPreviewCallback preview_callback = {},
               std::stop_token stop_token = {});

/** Publish an explicitly selected monitor as a video-only screen share. */
[[nodiscard]] int
publish_display(HMONITOR monitor, std::uint32_t frames_per_second,
                std::uint32_t maximum_width, std::uint32_t maximum_height,
                const PublisherCredential &credential,
                EncodedPreviewCallback preview_callback = {},
                std::stop_token stop_token = {});

} // namespace chatto::capture
