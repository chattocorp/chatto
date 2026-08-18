// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#include "preview_window.h"

#include <algorithm>
#include <iomanip>
#include <sstream>
#include <stdexcept>

namespace chatto::capture {
namespace {

constexpr wchar_t kPreviewWindowClass[] =
    L"ChattoWindowsCaptureProbePreviewWindow";
constexpr wchar_t kPreviewWindowTitle[] =
    L"Chatto Windows Capture Probe Preview";

[[nodiscard]] ATOM register_preview_window_class() {
  static const ATOM window_class = [] {
    WNDCLASSEXW description{};
    description.cbSize = sizeof(description);
    description.hInstance = GetModuleHandleW(nullptr);
    description.lpfnWndProc = PreviewWindow::window_procedure;
    description.lpszClassName = kPreviewWindowClass;
    description.hCursor = LoadCursorW(nullptr, IDC_ARROW);
    description.hbrBackground =
        reinterpret_cast<HBRUSH>(GetStockObject(BLACK_BRUSH));
    const ATOM registered = RegisterClassExW(&description);
    if (registered == 0 && GetLastError() != ERROR_CLASS_ALREADY_EXISTS) {
      winrt::throw_last_error();
    }
    return registered;
  }();
  return window_class;
}

[[nodiscard]] SIZE bounded_preview_size(
    const std::uint32_t width,
    const std::uint32_t height) {
  RECT work_area{};
  if (!SystemParametersInfoW(SPI_GETWORKAREA, 0, &work_area, 0)) {
    return SIZE{static_cast<LONG>(width), static_cast<LONG>(height)};
  }
  const double available_width =
      static_cast<double>(work_area.right - work_area.left) * 0.8;
  const double available_height =
      static_cast<double>(work_area.bottom - work_area.top) * 0.8;
  const double scale = std::min(
      {1.0,
       available_width / static_cast<double>(width),
       available_height / static_cast<double>(height)});
  return SIZE{
      static_cast<LONG>(static_cast<double>(width) * scale),
      static_cast<LONG>(static_cast<double>(height) * scale),
  };
}

}  // namespace

PreviewWindow::PreviewWindow(
    ID3D11Device& device,
    ID3D11DeviceContext& context,
    const std::uint32_t width,
    const std::uint32_t height)
    : width_(width), height_(height) {
  static_cast<void>(register_preview_window_class());
  winrt::check_hresult(context.QueryInterface(
      __uuidof(ID3D11DeviceContext), context_.put_void()));

  const SIZE client_size = bounded_preview_size(width, height);
  RECT bounds{0, 0, client_size.cx, client_size.cy};
  winrt::check_bool(AdjustWindowRectEx(
      &bounds, WS_OVERLAPPEDWINDOW, FALSE, 0));
  window_ = CreateWindowExW(
      0,
      kPreviewWindowClass,
      kPreviewWindowTitle,
      WS_OVERLAPPEDWINDOW,
      CW_USEDEFAULT,
      CW_USEDEFAULT,
      bounds.right - bounds.left,
      bounds.bottom - bounds.top,
      nullptr,
      nullptr,
      GetModuleHandleW(nullptr),
      this);
  if (window_ == nullptr) {
    winrt::throw_last_error();
  }

  winrt::com_ptr<IDXGIDevice> dxgi_device;
  winrt::check_hresult(
      device.QueryInterface(__uuidof(IDXGIDevice), dxgi_device.put_void()));
  winrt::com_ptr<IDXGIAdapter> adapter;
  winrt::check_hresult(dxgi_device->GetAdapter(adapter.put()));
  winrt::com_ptr<IDXGIFactory2> factory;
  winrt::check_hresult(adapter->GetParent(__uuidof(IDXGIFactory2), factory.put_void()));

  DXGI_SWAP_CHAIN_DESC1 description{};
  description.Width = width;
  description.Height = height;
  description.Format = DXGI_FORMAT_B8G8R8A8_UNORM;
  description.SampleDesc.Count = 1;
  description.BufferUsage = DXGI_USAGE_RENDER_TARGET_OUTPUT;
  description.BufferCount = 2;
  description.Scaling = DXGI_SCALING_STRETCH;
  description.SwapEffect = DXGI_SWAP_EFFECT_FLIP_DISCARD;
  description.AlphaMode = DXGI_ALPHA_MODE_IGNORE;
  winrt::check_hresult(factory->CreateSwapChainForHwnd(
      &device,
      window_,
      &description,
      nullptr,
      nullptr,
      swap_chain_.put()));
  winrt::check_hresult(factory->MakeWindowAssociation(window_, DXGI_MWA_NO_ALT_ENTER));
  winrt::check_hresult(
      swap_chain_->GetBuffer(0, __uuidof(ID3D11Texture2D), back_buffer_.put_void()));

  ShowWindow(window_, SW_SHOWNORMAL);
  UpdateWindow(window_);
}

PreviewWindow::~PreviewWindow() {
  std::scoped_lock lock(mutex_);
  back_buffer_ = nullptr;
  swap_chain_ = nullptr;
  if (window_ != nullptr && !closed_) {
    DestroyWindow(window_);
    window_ = nullptr;
  }
}

void PreviewWindow::present(ID3D11Texture2D& texture) {
  std::scoped_lock lock(mutex_);
  if (closed_ || window_ == nullptr) {
    return;
  }

  D3D11_TEXTURE2D_DESC description{};
  texture.GetDesc(&description);
  if (description.Width != width_ || description.Height != height_) {
    resize_swap_chain(description.Width, description.Height);
    resize_window(description.Width, description.Height);
  }

  context_->CopyResource(back_buffer_.get(), &texture);
  const HRESULT result = swap_chain_->Present(0, DXGI_PRESENT_DO_NOT_WAIT);
  if (result != DXGI_ERROR_WAS_STILL_DRAWING) {
    winrt::check_hresult(result);
  }
}

void PreviewWindow::pump_messages() {
  MSG message{};
  while (PeekMessageW(&message, window_, 0, 0, PM_REMOVE)) {
    TranslateMessage(&message);
    DispatchMessageW(&message);
  }
}

void PreviewWindow::update_status(
    const std::uint64_t frames,
    const std::uint64_t sampled_frames,
    const std::uint64_t changed_samples,
    const double observed_frames_per_second,
    const std::uint64_t audio_frames,
    const float latest_audio_peak,
    const std::uint64_t audio_discontinuities) {
  if (closed_ || window_ == nullptr) {
    return;
  }
  std::wostringstream title;
  title << kPreviewWindowTitle << L" — " << std::fixed << std::setprecision(1)
        << observed_frames_per_second << L" fps | frames " << frames
        << L" | samples " << changed_samples << L"/" << sampled_frames
        << L" changed | audio " << std::setprecision(3) << latest_audio_peak
        << L" peak (" << audio_frames << L" frames, "
        << audio_discontinuities << L" gaps)";
  SetWindowTextW(window_, title.str().c_str());
}

bool PreviewWindow::closed() const noexcept {
  return closed_;
}

LRESULT CALLBACK PreviewWindow::window_procedure(
    HWND window,
    const UINT message,
    const WPARAM word_parameter,
    const LPARAM long_parameter) {
  auto* preview = reinterpret_cast<PreviewWindow*>(
      GetWindowLongPtrW(window, GWLP_USERDATA));
  if (message == WM_NCCREATE) {
    const auto* creation = reinterpret_cast<CREATESTRUCTW*>(long_parameter);
    preview = static_cast<PreviewWindow*>(creation->lpCreateParams);
    SetWindowLongPtrW(
        window, GWLP_USERDATA, reinterpret_cast<LONG_PTR>(preview));
  }
  if (message == WM_CLOSE) {
    DestroyWindow(window);
    return 0;
  }
  if (message == WM_DESTROY && preview != nullptr) {
    preview->closed_ = true;
    return 0;
  }
  return DefWindowProcW(window, message, word_parameter, long_parameter);
}

void PreviewWindow::resize_swap_chain(
    const std::uint32_t width,
    const std::uint32_t height) {
  back_buffer_ = nullptr;
  winrt::check_hresult(swap_chain_->ResizeBuffers(
      0, width, height, DXGI_FORMAT_UNKNOWN, 0));
  winrt::check_hresult(
      swap_chain_->GetBuffer(0, __uuidof(ID3D11Texture2D), back_buffer_.put_void()));
  width_ = width;
  height_ = height;
}

void PreviewWindow::resize_window(
    const std::uint32_t width,
    const std::uint32_t height) {
  const SIZE client_size = bounded_preview_size(width, height);
  RECT bounds{0, 0, client_size.cx, client_size.cy};
  winrt::check_bool(AdjustWindowRectEx(
      &bounds, WS_OVERLAPPEDWINDOW, FALSE, 0));
  SetWindowPos(
      window_,
      nullptr,
      0,
      0,
      bounds.right - bounds.left,
      bounds.bottom - bounds.top,
      SWP_NOMOVE | SWP_NOACTIVATE | SWP_NOZORDER);
}

}  // namespace chatto::capture
