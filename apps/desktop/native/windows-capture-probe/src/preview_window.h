// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#pragma once

#include <atomic>
#include <cstdint>
#include <mutex>
#include <string>

#include <d3d11.h>
#include <dxgi1_2.h>
#include <windows.h>
#include <winrt/base.h>

namespace chatto::capture {

// Displays captured GPU textures directly through a flip-model DXGI swap chain.
// The preview owns no capture source identifiers and never reads pixels to disk.
class PreviewWindow {
 public:
  PreviewWindow(
      ID3D11Device& device,
      ID3D11DeviceContext& context,
      std::uint32_t width,
      std::uint32_t height);
  ~PreviewWindow();

  PreviewWindow(const PreviewWindow&) = delete;
  PreviewWindow& operator=(const PreviewWindow&) = delete;

  void present(ID3D11Texture2D& texture);
  void pump_messages();
  void update_status(
      std::uint64_t frames,
      std::uint64_t sampled_frames,
      std::uint64_t changed_samples,
      double observed_frames_per_second,
      std::uint64_t audio_frames,
      float latest_audio_peak,
      std::uint64_t audio_discontinuities);

  [[nodiscard]] bool closed() const noexcept;

  static LRESULT CALLBACK window_procedure(
      HWND window,
      UINT message,
      WPARAM word_parameter,
      LPARAM long_parameter);

 private:
  void resize_swap_chain(std::uint32_t width, std::uint32_t height);
  void resize_window(std::uint32_t width, std::uint32_t height);

  std::mutex mutex_;
  std::atomic_bool closed_ = false;
  HWND window_ = nullptr;
  winrt::com_ptr<ID3D11DeviceContext> context_;
  winrt::com_ptr<IDXGISwapChain1> swap_chain_;
  winrt::com_ptr<ID3D11Texture2D> back_buffer_;
  std::uint32_t width_ = 0;
  std::uint32_t height_ = 0;
};

}  // namespace chatto::capture
