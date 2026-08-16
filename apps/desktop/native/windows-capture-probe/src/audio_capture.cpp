// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#include "audio_capture.h"

#include "live_status.h"

#include <algorithm>
#include <cmath>
#include <limits>
#include <stdexcept>

#include <audioclient.h>
#include <audioclientactivationparams.h>
#include <ksmedia.h>
#include <mmdeviceapi.h>
#include <wrl.h>
#include <wrl/implements.h>
#include <winrt/base.h>

namespace chatto::capture {
namespace {

using Microsoft::WRL::ComPtr;
using Microsoft::WRL::FtmBase;
using Microsoft::WRL::Make;
using Microsoft::WRL::RuntimeClass;
using Microsoft::WRL::RuntimeClassFlags;
using Microsoft::WRL::ClassicCom;

class ActivationHandler final
    : public RuntimeClass<
          RuntimeClassFlags<ClassicCom>,
          FtmBase,
          IActivateAudioInterfaceCompletionHandler> {
 public:
  ActivationHandler() : completed_(CreateEventW(nullptr, FALSE, FALSE, nullptr)) {}

  ~ActivationHandler() override {
    if (completed_ != nullptr) {
      CloseHandle(completed_);
    }
  }

  HRESULT RuntimeClassInitialize() noexcept {
    return completed_ == nullptr ? HRESULT_FROM_WIN32(GetLastError()) : S_OK;
  }

  STDMETHODIMP ActivateCompleted(
      IActivateAudioInterfaceAsyncOperation* operation) noexcept override {
    HRESULT activation_result = E_UNEXPECTED;
    ComPtr<IUnknown> activated_interface;
    result_ = operation->GetActivateResult(
        &activation_result, activated_interface.GetAddressOf());
    if (SUCCEEDED(result_)) {
      result_ = activation_result;
    }
    if (SUCCEEDED(result_)) {
      result_ = activated_interface.As(&audio_client_);
    }
    SetEvent(completed_);
    return S_OK;
  }

  [[nodiscard]] ComPtr<IAudioClient> wait_for_client() {
    const DWORD wait_result = WaitForSingleObject(completed_, 30'000);
    if (wait_result != WAIT_OBJECT_0) {
      if (wait_result == WAIT_FAILED) {
        winrt::throw_last_error();
      }
      throw std::runtime_error("Timed out activating process-loopback audio");
    }
    winrt::check_hresult(result_);
    return audio_client_;
  }

 private:
  HANDLE completed_ = nullptr;
  HRESULT result_ = E_PENDING;
  ComPtr<IAudioClient> audio_client_;
};

[[nodiscard]] WAVEFORMATEXTENSIBLE capture_format() {
  WAVEFORMATEXTENSIBLE format{};
  format.Format.wFormatTag = WAVE_FORMAT_EXTENSIBLE;
  format.Format.nChannels = 2;
  format.Format.nSamplesPerSec = 48'000;
  format.Format.wBitsPerSample = 32;
  format.Format.nBlockAlign =
      format.Format.nChannels * format.Format.wBitsPerSample / 8;
  format.Format.nAvgBytesPerSec =
      format.Format.nSamplesPerSec * format.Format.nBlockAlign;
  format.Format.cbSize = sizeof(WAVEFORMATEXTENSIBLE) - sizeof(WAVEFORMATEX);
  format.Samples.wValidBitsPerSample = 32;
  format.dwChannelMask = SPEAKER_FRONT_LEFT | SPEAKER_FRONT_RIGHT;
  format.SubFormat = KSDATAFORMAT_SUBTYPE_IEEE_FLOAT;
  return format;
}

[[nodiscard]] ComPtr<IAudioClient> activate_process_loopback(DWORD process_id) {
  AUDIOCLIENT_ACTIVATION_PARAMS activation{};
  activation.ActivationType = AUDIOCLIENT_ACTIVATION_TYPE_PROCESS_LOOPBACK;
  activation.ProcessLoopbackParams.TargetProcessId = process_id;
  activation.ProcessLoopbackParams.ProcessLoopbackMode =
      PROCESS_LOOPBACK_MODE_INCLUDE_TARGET_PROCESS_TREE;

  PROPVARIANT parameters{};
  parameters.vt = VT_BLOB;
  parameters.blob.cbSize = sizeof(activation);
  parameters.blob.pBlobData = reinterpret_cast<BYTE*>(&activation);

  const auto handler = Make<ActivationHandler>();
  if (!handler) {
    throw std::bad_alloc();
  }
  winrt::check_hresult(handler->RuntimeClassInitialize());

  ComPtr<IActivateAudioInterfaceAsyncOperation> operation;
  winrt::check_hresult(ActivateAudioInterfaceAsync(
      VIRTUAL_AUDIO_DEVICE_PROCESS_LOOPBACK,
      __uuidof(IAudioClient),
      &parameters,
      handler.Get(),
      operation.GetAddressOf()));
  return handler->wait_for_client();
}

void consume_audio_packet(
    IAudioCaptureClient& capture_client,
    const WAVEFORMATEXTENSIBLE& format,
    AudioCaptureMetrics& metrics,
    std::uint64_t& first_timestamp,
    std::uint64_t& last_timestamp,
    const std::shared_ptr<LiveCaptureStatus>& live_status,
    const AudioFrameHandler& frame_handler) {
  BYTE* bytes = nullptr;
  UINT32 frames = 0;
  DWORD flags = 0;
  UINT64 device_position = 0;
  UINT64 qpc_position = 0;
  winrt::check_hresult(capture_client.GetBuffer(
      &bytes, &frames, &flags, &device_position, &qpc_position));

  metrics.packets += 1;
  metrics.frames += frames;
  if ((flags & AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY) != 0) {
    metrics.discontinuities += 1;
  }
  if ((flags & AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR) != 0) {
    metrics.timestamp_errors += 1;
  } else {
    if (first_timestamp == 0) {
      first_timestamp = qpc_position;
    }
    const auto packet_duration =
        static_cast<std::uint64_t>(frames) * 10'000'000ULL /
        format.Format.nSamplesPerSec;
    last_timestamp = qpc_position + packet_duration;
  }

  float packet_peak = 0;
  if ((flags & AUDCLNT_BUFFERFLAGS_SILENT) != 0 || bytes == nullptr) {
    metrics.silent_packets += 1;
  } else {
    const auto* samples = reinterpret_cast<const float*>(bytes);
    const std::size_t sample_count =
        static_cast<std::size_t>(frames) * format.Format.nChannels;
    for (std::size_t index = 0; index < sample_count; ++index) {
      packet_peak = std::max(packet_peak, std::abs(samples[index]));
    }
    metrics.peak_level = std::max(metrics.peak_level, packet_peak);
  }
  if (live_status) {
    live_status->audio_frames.store(metrics.frames, std::memory_order_relaxed);
    live_status->audio_discontinuities.store(
        metrics.discontinuities, std::memory_order_relaxed);
    live_status->latest_audio_peak.store(packet_peak, std::memory_order_relaxed);
  }
  if (frame_handler && frames > 0) {
    frame_handler(AudioFrameData{
        .sample_rate = format.Format.nSamplesPerSec,
        .channels = format.Format.nChannels,
        .frames = frames,
        .timestamp_100ns = qpc_position,
        .samples = reinterpret_cast<const float*>(bytes),
        .silent = (flags & AUDCLNT_BUFFERFLAGS_SILENT) != 0 || bytes == nullptr,
    });
  }

  winrt::check_hresult(capture_client.ReleaseBuffer(frames));
}

}  // namespace

AudioCaptureMetrics capture_process_audio(
    const DWORD process_id,
    const std::chrono::seconds duration,
    const std::stop_token stop_token,
    const std::shared_ptr<LiveCaptureStatus> live_status,
    AudioFrameHandler frame_handler) {
  if (process_id == 0 || duration.count() <= 0) {
    throw std::invalid_argument("Audio process and duration must be positive");
  }

  const auto audio_client = activate_process_loopback(process_id);
  const auto format = capture_format();
  constexpr DWORD stream_flags =
      AUDCLNT_STREAMFLAGS_LOOPBACK |
      AUDCLNT_STREAMFLAGS_EVENTCALLBACK |
      AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM |
      AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY;
  winrt::check_hresult(audio_client->Initialize(
      AUDCLNT_SHAREMODE_SHARED,
      stream_flags,
      0,
      0,
      &format.Format,
      nullptr));

  ComPtr<IAudioCaptureClient> capture_client;
  winrt::check_hresult(audio_client->GetService(IID_PPV_ARGS(&capture_client)));
  const HANDLE sample_ready = CreateEventW(nullptr, FALSE, FALSE, nullptr);
  if (sample_ready == nullptr) {
    winrt::throw_last_error();
  }

  try {
    winrt::check_hresult(audio_client->SetEventHandle(sample_ready));
    winrt::check_hresult(audio_client->Start());

    AudioCaptureMetrics metrics;
    metrics.sample_rate = format.Format.nSamplesPerSec;
    metrics.channels = format.Format.nChannels;
    std::uint64_t first_timestamp = 0;
    std::uint64_t last_timestamp = 0;
    const auto deadline = std::chrono::steady_clock::now() + duration;

    while (std::chrono::steady_clock::now() < deadline &&
           !stop_token.stop_requested()) {
      const auto remaining = std::chrono::duration_cast<std::chrono::milliseconds>(
          deadline - std::chrono::steady_clock::now());
      const DWORD timeout = static_cast<DWORD>(std::clamp<std::int64_t>(
          remaining.count(), 1, 1'000));
      const DWORD wait_result = WaitForSingleObject(sample_ready, timeout);
      if (wait_result == WAIT_FAILED) {
        winrt::throw_last_error();
      }
      if (wait_result != WAIT_OBJECT_0) {
        continue;
      }

      UINT32 available_frames = 0;
      winrt::check_hresult(capture_client->GetNextPacketSize(&available_frames));
      while (available_frames > 0) {
        consume_audio_packet(
            *capture_client.Get(),
            format,
            metrics,
            first_timestamp,
            last_timestamp,
            live_status,
            frame_handler);
        winrt::check_hresult(capture_client->GetNextPacketSize(&available_frames));
      }
    }

    winrt::check_hresult(audio_client->Stop());
    CloseHandle(sample_ready);
    if (first_timestamp != 0 && last_timestamp >= first_timestamp) {
      metrics.first_timestamp_100ns = first_timestamp;
      metrics.last_timestamp_100ns = last_timestamp;
      metrics.timestamp_span_seconds =
          static_cast<double>(last_timestamp - first_timestamp) / 10'000'000.0;
    }
    return metrics;
  } catch (...) {
    audio_client->Stop();
    CloseHandle(sample_ready);
    throw;
  }
}

}  // namespace chatto::capture
