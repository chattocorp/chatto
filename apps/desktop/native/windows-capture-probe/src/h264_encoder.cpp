// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#include "h264_encoder.h"
#include "video_frame_scaler.h"

#include <algorithm>
#include <array>
#include <cstdio>
#include <limits>
#include <stdexcept>
#include <utility>

#include <codecapi.h>
#include <dxgi.h>
#include <icodecapi.h>
#include <mfapi.h>
#include <mferror.h>
#include <mfidl.h>
#include <mftransform.h>
#include <wrl/client.h>

namespace chatto::capture {
namespace {

using Microsoft::WRL::ComPtr;

[[noreturn]] void throw_hresult(const char *message, const HRESULT result) {
  throw std::runtime_error(std::string(message) + " (0x" + [&result] {
    std::array<char, 9> text{};
    static_cast<void>(std::snprintf(text.data(), text.size(), "%08x",
                                    static_cast<unsigned int>(result)));
    return std::string(text.data());
  }() + ")");
}

void check_hresult(const HRESULT result, const char *message) {
  if (FAILED(result)) {
    throw_hresult(message, result);
  }
}

[[nodiscard]] std::uint8_t clamp_byte(const int value) {
  return static_cast<std::uint8_t>(std::clamp(value, 0, 255));
}

[[nodiscard]] bool
starts_with_start_code(const std::span<const std::uint8_t> bytes,
                       const std::size_t offset, std::size_t &start_code_size) {
  if (offset + 3 <= bytes.size() && bytes[offset] == 0 &&
      bytes[offset + 1] == 0 && bytes[offset + 2] == 1) {
    start_code_size = 3;
    return true;
  }
  if (offset + 4 <= bytes.size() && bytes[offset] == 0 &&
      bytes[offset + 1] == 0 && bytes[offset + 2] == 0 &&
      bytes[offset + 3] == 1) {
    start_code_size = 4;
    return true;
  }
  return false;
}

void normalize_h264_access_unit(std::vector<std::uint8_t> &access_unit) {
  std::size_t start_code_size = 0;
  if (starts_with_start_code(access_unit, 0, start_code_size)) {
    return;
  }

  std::vector<std::uint8_t> annex_b;
  std::size_t offset = 0;
  while (offset + 4 <= access_unit.size()) {
    const auto nal_size =
        (static_cast<std::uint32_t>(access_unit[offset]) << 24U) |
        (static_cast<std::uint32_t>(access_unit[offset + 1]) << 16U) |
        (static_cast<std::uint32_t>(access_unit[offset + 2]) << 8U) |
        static_cast<std::uint32_t>(access_unit[offset + 3]);
    offset += 4;
    if (nal_size == 0 || nal_size > access_unit.size() - offset) {
      return;
    }
    annex_b.insert(annex_b.end(), {0, 0, 0, 1});
    annex_b.insert(annex_b.end(), access_unit.begin() + offset,
                   access_unit.begin() + offset + nal_size);
    offset += nal_size;
  }
  if (offset == access_unit.size() && !annex_b.empty()) {
    access_unit = std::move(annex_b);
  }
}

void set_codec_u32(ICodecAPI *codec_api, const GUID &property,
                   const std::uint32_t value, const bool required = false) {
  if (codec_api == nullptr) {
    if (required) {
      throw std::runtime_error("The hardware H.264 encoder has no codec API");
    }
    return;
  }
  VARIANT setting;
  VariantInit(&setting);
  setting.vt = VT_UI4;
  setting.ulVal = value;
  const HRESULT result = codec_api->SetValue(&property, &setting);
  VariantClear(&setting);
  if (required && FAILED(result)) {
    throw_hresult("The hardware H.264 encoder rejected a required setting",
                  result);
  }
}

void set_codec_bool(ICodecAPI *codec_api, const GUID &property,
                    const bool value) {
  if (codec_api == nullptr) {
    return;
  }
  VARIANT setting;
  VariantInit(&setting);
  setting.vt = VT_BOOL;
  setting.boolVal = value ? VARIANT_TRUE : VARIANT_FALSE;
  static_cast<void>(codec_api->SetValue(&property, &setting));
  VariantClear(&setting);
}

[[nodiscard]] std::optional<std::uint32_t> get_codec_u32(ICodecAPI *codec_api,
                                                         const GUID &property) {
  if (codec_api == nullptr) {
    return std::nullopt;
  }
  VARIANT setting;
  VariantInit(&setting);
  const HRESULT result = codec_api->GetValue(&property, &setting);
  const auto value = SUCCEEDED(result) && setting.vt == VT_UI4
                         ? std::optional<std::uint32_t>(setting.ulVal)
                         : std::nullopt;
  VariantClear(&setting);
  return value;
}

[[nodiscard]] ComPtr<IMFMediaType>
make_video_type(const GUID &subtype, const std::uint32_t width,
                const std::uint32_t height,
                const std::uint32_t frames_per_second) {
  ComPtr<IMFMediaType> type;
  check_hresult(MFCreateMediaType(&type),
                "Could not create a video media type");
  check_hresult(type->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video),
                "Could not set the video major type");
  check_hresult(type->SetGUID(MF_MT_SUBTYPE, subtype),
                "Could not set the video subtype");
  check_hresult(MFSetAttributeSize(type.Get(), MF_MT_FRAME_SIZE, width, height),
                "Could not set the video frame size");
  check_hresult(
      MFSetAttributeRatio(type.Get(), MF_MT_FRAME_RATE, frames_per_second, 1),
      "Could not set the video frame rate");
  check_hresult(MFSetAttributeRatio(type.Get(), MF_MT_PIXEL_ASPECT_RATIO, 1, 1),
                "Could not set the video pixel aspect ratio");
  check_hresult(
      type->SetUINT32(MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive),
      "Could not set progressive video");
  return type;
}

[[nodiscard]] std::string narrow_name(IMFActivate *activation) {
  wchar_t *wide_name = nullptr;
  std::uint32_t length = 0;
  if (FAILED(activation->GetAllocatedString(MFT_FRIENDLY_NAME_Attribute,
                                            &wide_name, &length)) ||
      wide_name == nullptr) {
    return "Media Foundation hardware H.264 encoder";
  }
  const int byte_count =
      WideCharToMultiByte(CP_UTF8, 0, wide_name, static_cast<int>(length),
                          nullptr, 0, nullptr, nullptr);
  std::string name(static_cast<std::size_t>(std::max(byte_count, 0)), '\0');
  if (byte_count > 0) {
    static_cast<void>(WideCharToMultiByte(CP_UTF8, 0, wide_name,
                                          static_cast<int>(length), name.data(),
                                          byte_count, nullptr, nullptr));
  }
  CoTaskMemFree(wide_name);
  return name;
}

} // namespace

std::vector<std::uint8_t> bgra_to_nv12(const std::span<const std::uint8_t> bgra,
                                       const std::uint32_t width,
                                       const std::uint32_t height) {
  const auto expected_size =
      static_cast<std::size_t>(width) * static_cast<std::size_t>(height) * 4;
  if (width == 0 || height == 0 || (width % 2) != 0 || (height % 2) != 0 ||
      bgra.size() != expected_size) {
    throw std::invalid_argument(
        "NV12 conversion requires a tightly packed, even-sized BGRA frame");
  }

  const auto luma_size =
      static_cast<std::size_t>(width) * static_cast<std::size_t>(height);
  std::vector<std::uint8_t> nv12(luma_size + luma_size / 2);
  auto *luma = nv12.data();
  auto *chroma = nv12.data() + luma_size;

  for (std::uint32_t y = 0; y < height; ++y) {
    for (std::uint32_t x = 0; x < width; ++x) {
      const auto offset = (static_cast<std::size_t>(y) * width + x) * 4;
      const int blue = bgra[offset];
      const int green = bgra[offset + 1];
      const int red = bgra[offset + 2];
      luma[static_cast<std::size_t>(y) * width + x] =
          clamp_byte(16 + ((47 * red + 157 * green + 16 * blue + 128) >> 8));
    }
  }

  for (std::uint32_t y = 0; y < height; y += 2) {
    for (std::uint32_t x = 0; x < width; x += 2) {
      int blue = 0;
      int green = 0;
      int red = 0;
      for (std::uint32_t dy = 0; dy < 2; ++dy) {
        for (std::uint32_t dx = 0; dx < 2; ++dx) {
          const auto offset =
              (static_cast<std::size_t>(y + dy) * width + x + dx) * 4;
          blue += bgra[offset];
          green += bgra[offset + 1];
          red += bgra[offset + 2];
        }
      }
      blue /= 4;
      green /= 4;
      red /= 4;
      const auto chroma_offset = static_cast<std::size_t>(y / 2) * width + x;
      chroma[chroma_offset] =
          clamp_byte(128 + ((-26 * red - 87 * green + 112 * blue + 128) >> 8));
      chroma[chroma_offset + 1] =
          clamp_byte(128 + ((112 * red - 102 * green - 10 * blue + 128) >> 8));
    }
  }
  return nv12;
}

bool h264_access_unit_is_key_frame(
    const std::span<const std::uint8_t> access_unit) {
  for (std::size_t offset = 0; offset < access_unit.size(); ++offset) {
    std::size_t start_code_size = 0;
    if (!starts_with_start_code(access_unit, offset, start_code_size)) {
      continue;
    }
    const auto nal_offset = offset + start_code_size;
    if (nal_offset < access_unit.size() &&
        (access_unit[nal_offset] & 0x1fU) == 5U) {
      return true;
    }
    offset = nal_offset;
  }
  return false;
}

bool h264_access_unit_is_annex_b(
    const std::span<const std::uint8_t> access_unit) {
  std::size_t start_code_size = 0;
  return starts_with_start_code(access_unit, 0, start_code_size);
}

std::optional<H264ProfileLevel> h264_access_unit_profile_level(
    const std::span<const std::uint8_t> access_unit) {
  for (std::size_t offset = 0; offset < access_unit.size(); ++offset) {
    std::size_t start_code_size = 0;
    if (!starts_with_start_code(access_unit, offset, start_code_size)) {
      continue;
    }
    const auto nal_offset = offset + start_code_size;
    if (nal_offset + 3 < access_unit.size() &&
        (access_unit[nal_offset] & 0x1fU) == 7U) {
      return H264ProfileLevel{
          .profile_idc = access_unit[nal_offset + 1],
          .profile_iop = access_unit[nal_offset + 2],
          .level_idc = access_unit[nal_offset + 3],
      };
    }
    offset = nal_offset;
  }
  return std::nullopt;
}

class MediaFoundationH264Encoder::Implementation final {
public:
  Implementation(const std::uint32_t width, const std::uint32_t height,
                 const std::uint32_t frames_per_second,
                 const std::uint32_t target_bitrate_bps)
      : width_(width), height_(height), frames_per_second_(frames_per_second),
        frame_duration_100ns_(10'000'000LL / frames_per_second) {
    if (width == 0 || height == 0 || (width % 2) != 0 || (height % 2) != 0 ||
        frames_per_second == 0 || target_bitrate_bps == 0) {
      throw std::invalid_argument(
          "The hardware H.264 encoder settings are invalid");
    }
    check_hresult(MFStartup(MF_VERSION, MFSTARTUP_FULL),
                  "Could not start Media Foundation");
    media_foundation_started_ = true;
    try {
      activate_encoder(target_bitrate_bps);
    } catch (...) {
      MFShutdown();
      media_foundation_started_ = false;
      throw;
    }
  }

  ~Implementation() {
    if (transform_) {
      static_cast<void>(
          transform_->ProcessMessage(MFT_MESSAGE_NOTIFY_END_OF_STREAM, 0));
      static_cast<void>(
          transform_->ProcessMessage(MFT_MESSAGE_NOTIFY_END_STREAMING, 0));
      static_cast<void>(
          transform_->ProcessMessage(MFT_MESSAGE_COMMAND_FLUSH, 0));
    }
    if (activation_) {
      static_cast<void>(activation_->ShutdownObject());
    }
    if (media_foundation_started_) {
      static_cast<void>(MFShutdown());
    }
  }

  [[nodiscard]] std::vector<EncodedH264AccessUnit>
  encode(const std::span<const std::uint8_t> bgra,
         const std::int64_t timestamp_us, const bool force_key_frame) {
    std::vector<EncodedH264AccessUnit> output;
    wait_for_input(output);

    if (force_key_frame) {
      set_codec_bool(codec_api_.Get(), CODECAPI_AVEncVideoForceKeyFrame, true);
    }
    const auto nv12 = bgra_to_nv12(bgra, width_, height_);
    ComPtr<IMFSample> sample;
    ComPtr<IMFMediaBuffer> buffer;
    check_hresult(MFCreateSample(&sample),
                  "Could not create an encoder sample");
    check_hresult(
        MFCreateMemoryBuffer(static_cast<DWORD>(nv12.size()), &buffer),
        "Could not create an encoder input buffer");
    BYTE *destination = nullptr;
    DWORD capacity = 0;
    check_hresult(buffer->Lock(&destination, &capacity, nullptr),
                  "Could not lock the encoder input buffer");
    std::copy(nv12.begin(), nv12.end(), destination);
    check_hresult(buffer->Unlock(),
                  "Could not unlock the encoder input buffer");
    check_hresult(buffer->SetCurrentLength(static_cast<DWORD>(nv12.size())),
                  "Could not size the encoder input buffer");
    check_hresult(sample->AddBuffer(buffer.Get()),
                  "Could not attach the encoder input buffer");
    check_hresult(sample->SetSampleTime(timestamp_us * 10),
                  "Could not timestamp the encoder input sample");
    check_hresult(sample->SetSampleDuration(frame_duration_100ns_),
                  "Could not set the encoder input duration");
    if (first_input_) {
      static_cast<void>(
          sample->SetUINT32(MFSampleExtension_Discontinuity, TRUE));
      first_input_ = false;
    }
    check_hresult(transform_->ProcessInput(0, sample.Get(), 0),
                  "The hardware H.264 encoder rejected an input frame");
    --input_requests_;
    pump_events(false, output);
    return output;
  }

  [[nodiscard]] std::vector<EncodedH264AccessUnit>
  encode_gpu(ID3D11Texture2D &bgra_texture, const std::uint32_t source_width,
             const std::uint32_t source_height, const std::int64_t timestamp_us,
             const bool force_key_frame) {
    D3D11_TEXTURE2D_DESC description{};
    bgra_texture.GetDesc(&description);
    if (description.Width != source_width ||
        description.Height != source_height ||
        description.Format != DXGI_FORMAT_B8G8R8A8_UNORM) {
      throw std::invalid_argument(
          "The Media Foundation fallback received an invalid GPU frame");
    }
    ComPtr<IDXGIKeyedMutex> keyed_mutex;
    check_hresult(bgra_texture.QueryInterface(IID_PPV_ARGS(&keyed_mutex)),
                  "The GPU frame has no keyed mutex");
    check_hresult(keyed_mutex->AcquireSync(1, 5'000),
                  "Could not acquire the GPU frame for fallback readback");
    ComPtr<ID3D11Device> device;
    bgra_texture.GetDevice(&device);
    ComPtr<ID3D11DeviceContext> context;
    device->GetImmediateContext(&context);
    D3D11_TEXTURE2D_DESC staging_description = description;
    staging_description.BindFlags = 0;
    staging_description.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
    staging_description.MiscFlags = 0;
    staging_description.Usage = D3D11_USAGE_STAGING;
    ComPtr<ID3D11Texture2D> staging;
    try {
      check_hresult(
          device->CreateTexture2D(&staging_description, nullptr, &staging),
          "Could not create the fallback readback texture");
      context->CopyResource(staging.Get(), &bgra_texture);
    } catch (...) {
      static_cast<void>(keyed_mutex->ReleaseSync(0));
      throw;
    }
    check_hresult(keyed_mutex->ReleaseSync(0),
                  "Could not release the GPU frame after fallback readback");
    D3D11_MAPPED_SUBRESOURCE mapped{};
    check_hresult(context->Map(staging.Get(), 0, D3D11_MAP_READ, 0, &mapped),
                  "Could not map the fallback readback texture");
    std::vector<std::uint8_t> bgra(static_cast<std::size_t>(source_width) *
                                   source_height * 4);
    const auto row_bytes = static_cast<std::size_t>(source_width) * 4;
    for (std::uint32_t row = 0; row < source_height; ++row) {
      std::copy_n(static_cast<const std::uint8_t *>(mapped.pData) +
                      static_cast<std::size_t>(row) * mapped.RowPitch,
                  row_bytes,
                  bgra.data() + static_cast<std::size_t>(row) * row_bytes);
    }
    context->Unmap(staging.Get(), 0);
    auto scaled = scale_bgra_frame(std::move(bgra), source_width, source_height,
                                   width_, height_);
    return encode(scaled, timestamp_us, force_key_frame);
  }

  void set_target_bitrate(const std::uint32_t target_bitrate_bps) {
    if (target_bitrate_bps == 0 || target_bitrate_bps == target_bitrate_bps_) {
      return;
    }
    set_codec_u32(codec_api_.Get(), CODECAPI_AVEncCommonMeanBitRate,
                  target_bitrate_bps);
    target_bitrate_bps_ =
        get_codec_u32(codec_api_.Get(), CODECAPI_AVEncCommonMeanBitRate)
            .value_or(target_bitrate_bps);
  }

  [[nodiscard]] std::uint32_t target_bitrate_bps() const noexcept {
    return target_bitrate_bps_;
  }

  [[nodiscard]] std::uint32_t rate_control_mode() const noexcept {
    return rate_control_mode_;
  }

  [[nodiscard]] std::vector<EncodedH264AccessUnit> finish() {
    if (finished_) {
      return {};
    }
    finished_ = true;
    check_hresult(
        transform_->ProcessMessage(MFT_MESSAGE_NOTIFY_END_OF_STREAM, 0),
        "Could not end the hardware encoder stream");
    check_hresult(transform_->ProcessMessage(MFT_MESSAGE_COMMAND_DRAIN, 0),
                  "Could not drain the hardware encoder");
    std::vector<EncodedH264AccessUnit> output;
    while (!drain_complete_) {
      pump_one_event(false, output);
    }
    return output;
  }

  [[nodiscard]] const std::string &implementation_name() const noexcept {
    return implementation_name_;
  }

private:
  void activate_encoder(const std::uint32_t target_bitrate_bps) {
    const MFT_REGISTER_TYPE_INFO input_type{MFMediaType_Video,
                                            MFVideoFormat_NV12};
    const MFT_REGISTER_TYPE_INFO output_type{MFMediaType_Video,
                                             MFVideoFormat_H264};
    IMFActivate **activations = nullptr;
    UINT32 activation_count = 0;
    check_hresult(
        MFTEnumEx(MFT_CATEGORY_VIDEO_ENCODER,
                  MFT_ENUM_FLAG_HARDWARE | MFT_ENUM_FLAG_SORTANDFILTER,
                  &input_type, &output_type, &activations, &activation_count),
        "Could not enumerate hardware H.264 encoders");
    if (activation_count == 0) {
      CoTaskMemFree(activations);
      throw std::runtime_error(
          "Windows did not report a hardware Media Foundation H.264 encoder");
    }
    activation_.Attach(activations[0]);
    for (UINT32 index = 1; index < activation_count; ++index) {
      activations[index]->Release();
    }
    CoTaskMemFree(activations);
    implementation_name_ = narrow_name(activation_.Get());
    check_hresult(activation_->ActivateObject(IID_PPV_ARGS(&transform_)),
                  "Could not activate the hardware H.264 encoder");

    ComPtr<IMFAttributes> attributes;
    if (SUCCEEDED(transform_->GetAttributes(&attributes))) {
      UINT32 asynchronous = FALSE;
      if (SUCCEEDED(attributes->GetUINT32(MF_TRANSFORM_ASYNC, &asynchronous)) &&
          asynchronous != FALSE) {
        check_hresult(attributes->SetUINT32(MF_TRANSFORM_ASYNC_UNLOCK, TRUE),
                      "Could not unlock the asynchronous H.264 encoder");
      }
      static_cast<void>(attributes->SetUINT32(MF_LOW_LATENCY, TRUE));
    }

    static_cast<void>(transform_.As(&codec_api_));
    set_codec_bool(codec_api_.Get(), CODECAPI_AVLowLatencyMode, true);
    // Rate-control mode is a static CodecAPI property. Microsoft requires it
    // to be set before SetOutputType so that the subsequent media-type change
    // activates the requested mode for this encoding session.
    set_codec_u32(codec_api_.Get(), CODECAPI_AVEncCommonRateControlMode,
                  eAVEncCommonRateControlMode_CBR, true);

    auto output = make_video_type(MFVideoFormat_H264, width_, height_,
                                  frames_per_second_);
    check_hresult(output->SetUINT32(MF_MT_AVG_BITRATE, target_bitrate_bps),
                  "Could not set the H.264 output bitrate");
    check_hresult(
        output->SetUINT32(MF_MT_MPEG2_PROFILE, eAVEncH264VProfile_Base),
        "Could not set the H.264 output profile");
    check_hresult(transform_->SetOutputType(0, output.Get(), 0),
                  "The hardware encoder rejected the H.264 output type");
    auto input = make_video_type(MFVideoFormat_NV12, width_, height_,
                                 frames_per_second_);
    check_hresult(input->SetUINT32(MF_MT_DEFAULT_STRIDE, width_),
                  "Could not set the NV12 input stride");
    check_hresult(transform_->SetInputType(0, input.Get(), 0),
                  "The hardware encoder rejected the NV12 input type");

    set_codec_u32(codec_api_.Get(), CODECAPI_AVEncCommonMeanBitRate,
                  target_bitrate_bps, true);
    set_codec_u32(codec_api_.Get(), CODECAPI_AVEncMPVGOPSize,
                  frames_per_second_ * 2);
    target_bitrate_bps_ = target_bitrate_bps;
    rate_control_mode_ =
        get_codec_u32(codec_api_.Get(), CODECAPI_AVEncCommonRateControlMode)
            .value_or(std::numeric_limits<std::uint32_t>::max());

    check_hresult(transform_.As(&events_),
                  "The hardware H.264 encoder is not asynchronous");
    check_hresult(
        transform_->ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0),
        "Could not begin hardware H.264 streaming");
    check_hresult(
        transform_->ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM, 0),
        "Could not start the hardware H.264 stream");
  }

  void wait_for_input(std::vector<EncodedH264AccessUnit> &output) {
    while (input_requests_ == 0) {
      pump_one_event(false, output);
    }
  }

  void pump_events(const bool blocking,
                   std::vector<EncodedH264AccessUnit> &output) {
    bool first = true;
    while (true) {
      const HRESULT result = pump_one_event(!blocking || !first, output);
      if (result == MF_E_NO_EVENTS_AVAILABLE) {
        break;
      }
      first = false;
      if (blocking) {
        break;
      }
    }
  }

  HRESULT pump_one_event(const bool no_wait,
                         std::vector<EncodedH264AccessUnit> &output) {
    ComPtr<IMFMediaEvent> event;
    const HRESULT result =
        events_->GetEvent(no_wait ? MF_EVENT_FLAG_NO_WAIT : 0, &event);
    if (result == MF_E_NO_EVENTS_AVAILABLE) {
      return result;
    }
    check_hresult(result, "Could not read a hardware encoder event");
    HRESULT event_status = S_OK;
    check_hresult(event->GetStatus(&event_status),
                  "Could not read the hardware encoder event status");
    check_hresult(event_status, "The hardware H.264 encoder reported an error");
    MediaEventType type = MEUnknown;
    check_hresult(event->GetType(&type),
                  "Could not identify a hardware encoder event");
    if (type == METransformNeedInput) {
      ++input_requests_;
    } else if (type == METransformHaveOutput) {
      take_output(output);
    } else if (type == METransformDrainComplete) {
      drain_complete_ = true;
    }
    return S_OK;
  }

  void take_output(std::vector<EncodedH264AccessUnit> &output) {
    MFT_OUTPUT_STREAM_INFO information{};
    check_hresult(transform_->GetOutputStreamInfo(0, &information),
                  "Could not query the hardware encoder output stream");
    ComPtr<IMFSample> sample;
    if ((information.dwFlags & MFT_OUTPUT_STREAM_PROVIDES_SAMPLES) == 0) {
      ComPtr<IMFMediaBuffer> buffer;
      check_hresult(MFCreateSample(&sample),
                    "Could not create a hardware encoder output sample");
      check_hresult(
          MFCreateMemoryBuffer(std::max<DWORD>(information.cbSize, 1), &buffer),
          "Could not create a hardware encoder output buffer");
      check_hresult(sample->AddBuffer(buffer.Get()),
                    "Could not attach a hardware encoder output buffer");
    }

    MFT_OUTPUT_DATA_BUFFER data{};
    data.dwStreamID = 0;
    data.pSample = sample.Get();
    DWORD status = 0;
    const HRESULT result = transform_->ProcessOutput(0, 1, &data, &status);
    if (data.pEvents != nullptr) {
      data.pEvents->Release();
    }
    if (result == MF_E_TRANSFORM_NEED_MORE_INPUT) {
      return;
    }
    check_hresult(result, "Could not read a hardware H.264 access unit");
    if (data.pSample != nullptr && sample.Get() != data.pSample) {
      sample = data.pSample;
      data.pSample->Release();
    }
    if (!sample) {
      throw std::runtime_error("The hardware H.264 encoder returned no sample");
    }

    ComPtr<IMFMediaBuffer> contiguous;
    check_hresult(sample->ConvertToContiguousBuffer(&contiguous),
                  "Could not combine the H.264 output buffers");
    BYTE *bytes = nullptr;
    DWORD length = 0;
    check_hresult(contiguous->Lock(&bytes, nullptr, &length),
                  "Could not lock the H.264 output buffer");
    EncodedH264AccessUnit access_unit;
    access_unit.data.assign(bytes, bytes + length);
    check_hresult(contiguous->Unlock(),
                  "Could not unlock the H.264 output buffer");
    LONGLONG timestamp_100ns = 0;
    if (SUCCEEDED(sample->GetSampleTime(&timestamp_100ns))) {
      access_unit.timestamp_us = timestamp_100ns / 10;
    }
    normalize_h264_access_unit(access_unit.data);
    if (!h264_access_unit_is_annex_b(access_unit.data)) {
      throw std::runtime_error(
          "The hardware H.264 encoder returned an unsupported bitstream");
    }
    UINT32 clean_point = FALSE;
    access_unit.key_frame = (SUCCEEDED(sample->GetUINT32(
                                 MFSampleExtension_CleanPoint, &clean_point)) &&
                             clean_point != FALSE) ||
                            h264_access_unit_is_key_frame(access_unit.data);
    if (!access_unit.data.empty()) {
      output.push_back(std::move(access_unit));
    }
  }

  std::uint32_t width_;
  std::uint32_t height_;
  std::uint32_t frames_per_second_;
  LONGLONG frame_duration_100ns_;
  std::uint32_t target_bitrate_bps_ = 0;
  std::uint32_t rate_control_mode_ = std::numeric_limits<std::uint32_t>::max();
  bool media_foundation_started_ = false;
  bool first_input_ = true;
  bool finished_ = false;
  bool drain_complete_ = false;
  std::uint32_t input_requests_ = 0;
  std::string implementation_name_;
  ComPtr<IMFActivate> activation_;
  ComPtr<IMFTransform> transform_;
  ComPtr<IMFMediaEventGenerator> events_;
  ComPtr<ICodecAPI> codec_api_;
};

MediaFoundationH264Encoder::MediaFoundationH264Encoder(
    const std::uint32_t width, const std::uint32_t height,
    const std::uint32_t frames_per_second,
    const std::uint32_t target_bitrate_bps)
    : implementation_(std::make_unique<Implementation>(
          width, height, frames_per_second, target_bitrate_bps)) {}

MediaFoundationH264Encoder::~MediaFoundationH264Encoder() = default;

std::vector<EncodedH264AccessUnit>
MediaFoundationH264Encoder::encode(const std::span<const std::uint8_t> bgra,
                                   const std::int64_t timestamp_us,
                                   const bool force_key_frame) {
  return implementation_->encode(bgra, timestamp_us, force_key_frame);
}

std::vector<EncodedH264AccessUnit> MediaFoundationH264Encoder::encode_gpu(
    ID3D11Texture2D &bgra_texture, const std::uint32_t source_width,
    const std::uint32_t source_height, const std::int64_t timestamp_us,
    const bool force_key_frame) {
  return implementation_->encode_gpu(bgra_texture, source_width, source_height,
                                     timestamp_us, force_key_frame);
}

void MediaFoundationH264Encoder::set_target_bitrate(
    const std::uint32_t target_bitrate_bps) {
  implementation_->set_target_bitrate(target_bitrate_bps);
}

std::uint32_t MediaFoundationH264Encoder::target_bitrate_bps() const noexcept {
  return implementation_->target_bitrate_bps();
}

std::uint32_t MediaFoundationH264Encoder::rate_control_mode() const noexcept {
  return implementation_->rate_control_mode();
}

std::vector<EncodedH264AccessUnit> MediaFoundationH264Encoder::finish() {
  return implementation_->finish();
}

const std::string &
MediaFoundationH264Encoder::implementation_name() const noexcept {
  return implementation_->implementation_name();
}

} // namespace chatto::capture
