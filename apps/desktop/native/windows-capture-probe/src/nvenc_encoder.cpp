// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#include "h264_encoder.h"

#include <array>
#include <chrono>
#include <cstdint>
#include <iostream>
#include <limits>
#include <memory>
#include <stdexcept>
#include <string>
#include <utility>

#include <d3d11.h>
#include <d3d11_1.h>
#include <d3d11_4.h>
#include <dxgi1_2.h>
#include <ffnvcodec/nvEncodeAPI.h>
#include <windows.h>
#include <wrl/client.h>

namespace chatto::capture {
namespace {

using Microsoft::WRL::ComPtr;

constexpr std::uint32_t kNvidiaVendorId = 0x10de;
constexpr std::size_t kSurfaceCount = 4;

class NvencError final : public std::runtime_error {
public:
  explicit NvencError(const std::string &message)
      : std::runtime_error(message) {}
};

class UniqueModule final {
public:
  explicit UniqueModule(const wchar_t *name) : module_(LoadLibraryW(name)) {
    if (module_ == nullptr) {
      throw NvencError("The NVIDIA NVENC driver library is unavailable");
    }
  }

  ~UniqueModule() {
    if (module_ != nullptr) {
      FreeLibrary(module_);
    }
  }

  UniqueModule(const UniqueModule &) = delete;
  UniqueModule &operator=(const UniqueModule &) = delete;

  [[nodiscard]] FARPROC function(const char *name) const {
    const auto address = GetProcAddress(module_, name);
    if (address == nullptr) {
      throw NvencError(std::string("The NVIDIA driver does not export ") +
                       name);
    }
    return address;
  }

private:
  HMODULE module_ = nullptr;
};

[[nodiscard]] std::string
nvenc_status_message(const NVENCSTATUS status,
                     const NV_ENCODE_API_FUNCTION_LIST &functions,
                     void *encoder) {
  std::string message = "NVENC error " + std::to_string(status);
  if (encoder != nullptr && functions.nvEncGetLastErrorString != nullptr) {
    if (const char *detail = functions.nvEncGetLastErrorString(encoder);
        detail != nullptr && detail[0] != '\0') {
      message += ": ";
      message += detail;
    }
  }
  return message;
}

void check_nvenc(const NVENCSTATUS status, const char *operation,
                 const NV_ENCODE_API_FUNCTION_LIST &functions,
                 void *encoder = nullptr) {
  if (status != NV_ENC_SUCCESS) {
    throw NvencError(std::string(operation) + " failed (" +
                     nvenc_status_message(status, functions, encoder) + ")");
  }
}

[[nodiscard]] ComPtr<IDXGIAdapter1> find_nvidia_adapter() {
  ComPtr<IDXGIFactory1> factory;
  if (FAILED(CreateDXGIFactory1(IID_PPV_ARGS(&factory)))) {
    throw NvencError("Could not create a DXGI factory for NVENC");
  }
  for (UINT index = 0;; ++index) {
    ComPtr<IDXGIAdapter1> adapter;
    if (factory->EnumAdapters1(index, &adapter) == DXGI_ERROR_NOT_FOUND) {
      break;
    }
    DXGI_ADAPTER_DESC1 description{};
    if (SUCCEEDED(adapter->GetDesc1(&description)) &&
        description.VendorId == kNvidiaVendorId &&
        (description.Flags & DXGI_ADAPTER_FLAG_SOFTWARE) == 0) {
      return adapter;
    }
  }
  throw NvencError("No NVIDIA graphics adapter is available");
}

class DirectNvencH264Encoder final : public H264Encoder {
public:
  DirectNvencH264Encoder(const std::uint32_t width, const std::uint32_t height,
                         const std::uint32_t frames_per_second,
                         const std::uint32_t target_bitrate_bps)
      : width_(width), height_(height), frames_per_second_(frames_per_second),
        target_bitrate_bps_(target_bitrate_bps), module_(L"nvEncodeAPI64.dll") {
    if (width == 0 || height == 0 || (width % 2) != 0 || (height % 2) != 0 ||
        frames_per_second == 0 || target_bitrate_bps == 0) {
      throw std::invalid_argument("The NVENC H.264 settings are invalid");
    }
    try {
      initialize_api();
      initialize_device();
      open_session();
      initialize_encoder();
      create_surfaces();
    } catch (...) {
      release();
      throw;
    }
  }

  ~DirectNvencH264Encoder() override { release(); }

  DirectNvencH264Encoder(const DirectNvencH264Encoder &) = delete;
  DirectNvencH264Encoder &operator=(const DirectNvencH264Encoder &) = delete;

  [[nodiscard]] std::vector<EncodedH264AccessUnit>
  encode(const std::span<const std::uint8_t> bgra,
         const std::int64_t timestamp_us, const bool force_key_frame) override {
    if (bgra.size() != static_cast<std::size_t>(width_) * height_ * 4) {
      throw std::invalid_argument("The NVENC BGRA test frame is invalid");
    }
    D3D11_TEXTURE2D_DESC description{};
    description.Width = width_;
    description.Height = height_;
    description.MipLevels = 1;
    description.ArraySize = 1;
    description.Format = DXGI_FORMAT_B8G8R8A8_UNORM;
    description.SampleDesc.Count = 1;
    description.Usage = D3D11_USAGE_DEFAULT;
    description.BindFlags = 0;
    description.MiscFlags = D3D11_RESOURCE_MISC_SHARED_KEYEDMUTEX;
    D3D11_SUBRESOURCE_DATA initial{};
    initial.pSysMem = bgra.data();
    initial.SysMemPitch = width_ * 4;
    ComPtr<ID3D11Texture2D> texture;
    if (FAILED(device_->CreateTexture2D(&description, &initial, &texture))) {
      throw NvencError("Could not upload the NVENC test frame");
    }
    ComPtr<IDXGIKeyedMutex> mutex;
    if (FAILED(texture.As(&mutex)) || FAILED(mutex->AcquireSync(0, INFINITE)) ||
        FAILED(mutex->ReleaseSync(1))) {
      throw NvencError("Could not publish the NVENC test frame");
    }
    return encode_gpu(*texture.Get(), width_, height_, timestamp_us,
                      force_key_frame);
  }

  [[nodiscard]] std::vector<EncodedH264AccessUnit>
  encode_gpu(ID3D11Texture2D &bgra_texture, const std::uint32_t source_width,
             const std::uint32_t source_height, const std::int64_t timestamp_us,
             const bool force_key_frame) override {
    if (finished_) {
      throw NvencError("NVENC cannot encode after the stream has finished");
    }
    auto &surface = surfaces_[surface_index_];
    surface_index_ = (surface_index_ + 1) % surfaces_.size();

    const auto conversion_start = std::chrono::steady_clock::now();
    convert_bgra_to_nv12(bgra_texture, source_width, source_height, surface);
    const auto conversion_end = std::chrono::steady_clock::now();
    last_gpu_conversion_submit_ms_ = std::chrono::duration<double, std::milli>(
                                         conversion_end - conversion_start)
                                         .count();

    const auto submit_start = std::chrono::steady_clock::now();
    NV_ENC_MAP_INPUT_RESOURCE map{};
    map.version = NV_ENC_MAP_INPUT_RESOURCE_VER;
    map.registeredResource = surface.registered;
    check_nvenc(functions_.nvEncMapInputResource(encoder_, &map),
                "Mapping an NVENC input texture", functions_, encoder_);

    NV_ENC_PIC_PARAMS picture{};
    picture.version = NV_ENC_PIC_PARAMS_VER;
    picture.inputWidth = width_;
    picture.inputHeight = height_;
    picture.inputPitch = width_;
    picture.inputBuffer = map.mappedResource;
    picture.bufferFmt = map.mappedBufferFmt;
    picture.outputBitstream = surface.bitstream;
    picture.inputTimeStamp = static_cast<std::uint64_t>(timestamp_us);
    picture.inputDuration = 1'000'000U / frames_per_second_;
    picture.pictureStruct = NV_ENC_PIC_STRUCT_FRAME;
    if (force_key_frame) {
      picture.encodePicFlags =
          NV_ENC_PIC_FLAG_FORCEIDR | NV_ENC_PIC_FLAG_OUTPUT_SPSPPS;
    }

    bool bitstream_locked = false;
    try {
      const NVENCSTATUS result =
          functions_.nvEncEncodePicture(encoder_, &picture);
      if (result == NV_ENC_ERR_NEED_MORE_INPUT) {
        static_cast<void>(
            functions_.nvEncUnmapInputResource(encoder_, map.mappedResource));
        return {};
      }
      check_nvenc(result, "Encoding an NVENC frame", functions_, encoder_);
      const auto submit_end = std::chrono::steady_clock::now();
      last_encoder_submit_ms_ =
          std::chrono::duration<double, std::milli>(submit_end - submit_start)
              .count();

      const auto wait_start = std::chrono::steady_clock::now();
      NV_ENC_LOCK_BITSTREAM lock{};
      lock.version = NV_ENC_LOCK_BITSTREAM_VER;
      lock.outputBitstream = surface.bitstream;
      lock.doNotWait = 0;
      check_nvenc(functions_.nvEncLockBitstream(encoder_, &lock),
                  "Locking an NVENC access unit", functions_, encoder_);
      bitstream_locked = true;
      last_bitstream_wait_ms_ =
          std::chrono::duration<double, std::milli>(
              std::chrono::steady_clock::now() - wait_start)
              .count();

      EncodedH264AccessUnit access_unit;
      const auto *bytes =
          static_cast<const std::uint8_t *>(lock.bitstreamBufferPtr);
      access_unit.data.assign(bytes, bytes + lock.bitstreamSizeInBytes);
      access_unit.timestamp_us =
          static_cast<std::int64_t>(lock.outputTimeStamp);
      access_unit.key_frame = lock.pictureType == NV_ENC_PIC_TYPE_IDR ||
                              lock.pictureType == NV_ENC_PIC_TYPE_I ||
                              h264_access_unit_is_key_frame(access_unit.data);

      const NVENCSTATUS unlock_result =
          functions_.nvEncUnlockBitstream(encoder_, surface.bitstream);
      bitstream_locked = false;
      const NVENCSTATUS unmap_result =
          functions_.nvEncUnmapInputResource(encoder_, map.mappedResource);
      map.mappedResource = nullptr;
      check_nvenc(unlock_result, "Unlocking an NVENC access unit", functions_,
                  encoder_);
      check_nvenc(unmap_result, "Unmapping an NVENC input texture", functions_,
                  encoder_);
      if (!h264_access_unit_is_annex_b(access_unit.data)) {
        throw NvencError("NVENC returned a non-Annex-B H.264 access unit");
      }
      return {std::move(access_unit)};
    } catch (...) {
      if (bitstream_locked) {
        static_cast<void>(
            functions_.nvEncUnlockBitstream(encoder_, surface.bitstream));
      }
      if (map.mappedResource != nullptr) {
        static_cast<void>(
            functions_.nvEncUnmapInputResource(encoder_, map.mappedResource));
      }
      throw;
    }
  }

  void set_target_bitrate(const std::uint32_t target_bitrate_bps) override {
    if (target_bitrate_bps == 0 || target_bitrate_bps == target_bitrate_bps_) {
      return;
    }
    config_.rcParams.averageBitRate = target_bitrate_bps;
    config_.rcParams.maxBitRate = target_bitrate_bps;
    config_.rcParams.vbvBufferSize =
        std::max(1U, target_bitrate_bps / frames_per_second_);
    config_.rcParams.vbvInitialDelay = config_.rcParams.vbvBufferSize;
    initialization_.encodeConfig = &config_;

    NV_ENC_RECONFIGURE_PARAMS reconfigure{};
    reconfigure.version = NV_ENC_RECONFIGURE_PARAMS_VER;
    reconfigure.reInitEncodeParams = initialization_;
    check_nvenc(functions_.nvEncReconfigureEncoder(encoder_, &reconfigure),
                "Reconfiguring the NVENC bitrate", functions_, encoder_);
    target_bitrate_bps_ = target_bitrate_bps;
  }

  [[nodiscard]] std::uint32_t target_bitrate_bps() const noexcept override {
    return target_bitrate_bps_;
  }

  [[nodiscard]] std::uint32_t rate_control_mode() const noexcept override {
    return static_cast<std::uint32_t>(config_.rcParams.rateControlMode);
  }

  [[nodiscard]] double last_gpu_conversion_submit_ms() const noexcept override {
    return last_gpu_conversion_submit_ms_;
  }

  [[nodiscard]] double last_encoder_submit_ms() const noexcept override {
    return last_encoder_submit_ms_;
  }

  [[nodiscard]] double last_bitstream_wait_ms() const noexcept override {
    return last_bitstream_wait_ms_;
  }

  [[nodiscard]] std::vector<EncodedH264AccessUnit> finish() override {
    if (finished_) {
      return {};
    }
    finished_ = true;
    NV_ENC_PIC_PARAMS picture{};
    picture.version = NV_ENC_PIC_PARAMS_VER;
    picture.encodePicFlags = NV_ENC_PIC_FLAG_EOS;
    check_nvenc(functions_.nvEncEncodePicture(encoder_, &picture),
                "Finishing the NVENC stream", functions_, encoder_);
    return {};
  }

  [[nodiscard]] const std::string &
  implementation_name() const noexcept override {
    return implementation_name_;
  }

private:
  struct Surface {
    ComPtr<ID3D11Texture2D> texture;
    ComPtr<ID3D11VideoProcessorOutputView> output_view;
    NV_ENC_REGISTERED_PTR registered = nullptr;
    NV_ENC_OUTPUT_PTR bitstream = nullptr;
  };

  void initialize_api() {
    using CreateInstance =
        NVENCSTATUS(NVENCAPI *)(NV_ENCODE_API_FUNCTION_LIST *);
    using GetMaxVersion = NVENCSTATUS(NVENCAPI *)(std::uint32_t *);
    const auto create_instance = reinterpret_cast<CreateInstance>(
        module_.function("NvEncodeAPICreateInstance"));
    const auto get_max_version = reinterpret_cast<GetMaxVersion>(
        module_.function("NvEncodeAPIGetMaxSupportedVersion"));

    std::uint32_t maximum_version = 0;
    NV_ENCODE_API_FUNCTION_LIST empty_functions{};
    check_nvenc(get_max_version(&maximum_version),
                "Querying the NVENC driver version", empty_functions);
    if (maximum_version < NVENCAPI_VERSION) {
      throw NvencError("The NVIDIA driver NVENC API is older than the pinned "
                       "Chatto headers (driver=" +
                       std::to_string(maximum_version >> 4U) + "." +
                       std::to_string(maximum_version & 0x0fU) +
                       ", headers=" + std::to_string(NVENCAPI_MAJOR_VERSION) +
                       "." + std::to_string(NVENCAPI_MINOR_VERSION) + ")");
    }
    functions_.version = NV_ENCODE_API_FUNCTION_LIST_VER;
    check_nvenc(create_instance(&functions_), "Loading the NVENC API",
                functions_);
  }

  void initialize_device() {
    adapter_ = find_nvidia_adapter();
    constexpr std::array feature_levels{
        D3D_FEATURE_LEVEL_12_1,
        D3D_FEATURE_LEVEL_12_0,
        D3D_FEATURE_LEVEL_11_1,
        D3D_FEATURE_LEVEL_11_0,
    };
    D3D_FEATURE_LEVEL selected_level{};
    const HRESULT result = D3D11CreateDevice(
        adapter_.Get(), D3D_DRIVER_TYPE_UNKNOWN, nullptr,
        D3D11_CREATE_DEVICE_BGRA_SUPPORT | D3D11_CREATE_DEVICE_VIDEO_SUPPORT,
        feature_levels.data(), static_cast<UINT>(feature_levels.size()),
        D3D11_SDK_VERSION, &device_, &selected_level, &context_);
    if (FAILED(result)) {
      throw NvencError("Could not create the NVIDIA D3D11 encoding device");
    }
    ComPtr<ID3D11Multithread> multithread;
    if (SUCCEEDED(context_.As(&multithread))) {
      multithread->SetMultithreadProtected(TRUE);
    }
    if (FAILED(device_.As(&video_device_)) ||
        FAILED(context_.As(&video_context_))) {
      throw NvencError("The NVIDIA device has no D3D11 video processor");
    }
    static_cast<void>(video_context_.As(&video_context1_));
  }

  void open_session() {
    NV_ENC_OPEN_ENCODE_SESSION_EX_PARAMS open{};
    open.version = NV_ENC_OPEN_ENCODE_SESSION_EX_PARAMS_VER;
    open.device = device_.Get();
    open.deviceType = NV_ENC_DEVICE_TYPE_DIRECTX;
    open.apiVersion = NVENCAPI_VERSION;
    check_nvenc(functions_.nvEncOpenEncodeSessionEx(&open, &encoder_),
                "Opening the NVENC session", functions_);
  }

  void initialize_encoder() {
    NV_ENC_PRESET_CONFIG preset{};
    preset.version = NV_ENC_PRESET_CONFIG_VER;
    preset.presetCfg.version = NV_ENC_CONFIG_VER;
    check_nvenc(functions_.nvEncGetEncodePresetConfigEx(
                    encoder_, NV_ENC_CODEC_H264_GUID, NV_ENC_PRESET_P5_GUID,
                    NV_ENC_TUNING_INFO_LOW_LATENCY, &preset),
                "Reading the NVENC low-latency preset", functions_, encoder_);
    config_ = preset.presetCfg;
    config_.version = NV_ENC_CONFIG_VER;
    // Our current LiveKit fork negotiates constrained-baseline H.264. Keep the
    // bitstream honest until the negotiated profile is returned to this helper.
    config_.profileGUID = NV_ENC_H264_PROFILE_BASELINE_GUID;
    config_.gopLength = frames_per_second_ * 2;
    config_.frameIntervalP = 1;
    config_.rcParams.rateControlMode = NV_ENC_PARAMS_RC_CBR;
    config_.rcParams.averageBitRate = target_bitrate_bps_;
    config_.rcParams.maxBitRate = target_bitrate_bps_;
    config_.rcParams.vbvBufferSize =
        std::max(1U, target_bitrate_bps_ / frames_per_second_);
    config_.rcParams.vbvInitialDelay = config_.rcParams.vbvBufferSize;
    config_.rcParams.multiPass = NV_ENC_TWO_PASS_QUARTER_RESOLUTION;
    config_.rcParams.enableAQ = 1;
    config_.rcParams.aqStrength = 8;
    config_.encodeCodecConfig.h264Config.idrPeriod = config_.gopLength;
    config_.encodeCodecConfig.h264Config.repeatSPSPPS = 1;
    config_.encodeCodecConfig.h264Config.entropyCodingMode =
        NV_ENC_H264_ENTROPY_CODING_MODE_CAVLC;

    initialization_.version = NV_ENC_INITIALIZE_PARAMS_VER;
    initialization_.encodeGUID = NV_ENC_CODEC_H264_GUID;
    initialization_.presetGUID = NV_ENC_PRESET_P5_GUID;
    initialization_.encodeWidth = width_;
    initialization_.encodeHeight = height_;
    initialization_.darWidth = width_;
    initialization_.darHeight = height_;
    initialization_.frameRateNum = frames_per_second_;
    initialization_.frameRateDen = 1;
    initialization_.enableEncodeAsync = 0;
    initialization_.enablePTD = 1;
    initialization_.encodeConfig = &config_;
    initialization_.maxEncodeWidth = width_;
    initialization_.maxEncodeHeight = height_;
    initialization_.tuningInfo = NV_ENC_TUNING_INFO_LOW_LATENCY;
    check_nvenc(functions_.nvEncInitializeEncoder(encoder_, &initialization_),
                "Initializing the NVENC H.264 encoder", functions_, encoder_);
  }

  void create_surfaces() {
    D3D11_TEXTURE2D_DESC texture_description{};
    texture_description.Width = width_;
    texture_description.Height = height_;
    texture_description.MipLevels = 1;
    texture_description.ArraySize = 1;
    texture_description.Format = DXGI_FORMAT_NV12;
    texture_description.SampleDesc.Count = 1;
    texture_description.Usage = D3D11_USAGE_DEFAULT;
    texture_description.BindFlags = D3D11_BIND_RENDER_TARGET;

    for (auto &surface : surfaces_) {
      if (FAILED(device_->CreateTexture2D(&texture_description, nullptr,
                                          &surface.texture))) {
        throw NvencError("Could not create an NVENC NV12 input texture");
      }
      NV_ENC_REGISTER_RESOURCE registration{};
      registration.version = NV_ENC_REGISTER_RESOURCE_VER;
      registration.resourceType = NV_ENC_INPUT_RESOURCE_TYPE_DIRECTX;
      registration.width = width_;
      registration.height = height_;
      registration.resourceToRegister = surface.texture.Get();
      registration.bufferFormat = NV_ENC_BUFFER_FORMAT_NV12;
      registration.bufferUsage = NV_ENC_INPUT_IMAGE;
      check_nvenc(functions_.nvEncRegisterResource(encoder_, &registration),
                  "Registering an NVENC input texture", functions_, encoder_);
      surface.registered = registration.registeredResource;

      NV_ENC_CREATE_BITSTREAM_BUFFER bitstream{};
      bitstream.version = NV_ENC_CREATE_BITSTREAM_BUFFER_VER;
      check_nvenc(functions_.nvEncCreateBitstreamBuffer(encoder_, &bitstream),
                  "Creating an NVENC output buffer", functions_, encoder_);
      surface.bitstream = bitstream.bitstreamBuffer;
    }
  }

  void ensure_video_processor(const std::uint32_t source_width,
                              const std::uint32_t source_height) {
    if (video_processor_ && source_width == processor_source_width_ &&
        source_height == processor_source_height_) {
      return;
    }
    for (auto &surface : surfaces_) {
      surface.output_view.Reset();
    }
    video_processor_.Reset();
    video_enumerator_.Reset();
    D3D11_VIDEO_PROCESSOR_CONTENT_DESC content{};
    content.InputFrameFormat = D3D11_VIDEO_FRAME_FORMAT_PROGRESSIVE;
    content.InputFrameRate = {frames_per_second_, 1};
    content.InputWidth = source_width;
    content.InputHeight = source_height;
    content.OutputFrameRate = {frames_per_second_, 1};
    content.OutputWidth = width_;
    content.OutputHeight = height_;
    content.Usage = D3D11_VIDEO_USAGE_OPTIMAL_QUALITY;
    if (FAILED(video_device_->CreateVideoProcessorEnumerator(
            &content, &video_enumerator_))) {
      throw NvencError("Could not enumerate NVIDIA video processing");
    }
    UINT input_flags = 0;
    UINT output_flags = 0;
    if (FAILED(video_enumerator_->CheckVideoProcessorFormat(
            DXGI_FORMAT_B8G8R8A8_UNORM, &input_flags)) ||
        (input_flags & D3D11_VIDEO_PROCESSOR_FORMAT_SUPPORT_INPUT) == 0 ||
        FAILED(video_enumerator_->CheckVideoProcessorFormat(DXGI_FORMAT_NV12,
                                                            &output_flags)) ||
        (output_flags & D3D11_VIDEO_PROCESSOR_FORMAT_SUPPORT_OUTPUT) == 0) {
      throw NvencError(
          "The NVIDIA video processor cannot convert BGRA to NV12");
    }
    if (FAILED(video_device_->CreateVideoProcessor(video_enumerator_.Get(), 0,
                                                   &video_processor_))) {
      throw NvencError("Could not create the NVIDIA video processor");
    }
    for (auto &surface : surfaces_) {
      D3D11_VIDEO_PROCESSOR_OUTPUT_VIEW_DESC view{};
      view.ViewDimension = D3D11_VPOV_DIMENSION_TEXTURE2D;
      view.Texture2D.MipSlice = 0;
      if (FAILED(video_device_->CreateVideoProcessorOutputView(
              surface.texture.Get(), video_enumerator_.Get(), &view,
              &surface.output_view))) {
        throw NvencError("Could not create an NV12 video processor output");
      }
    }
    processor_source_width_ = source_width;
    processor_source_height_ = source_height;
  }

  void convert_bgra_to_nv12(ID3D11Texture2D &source,
                            const std::uint32_t source_width,
                            const std::uint32_t source_height,
                            Surface &surface) {
    D3D11_TEXTURE2D_DESC source_description{};
    source.GetDesc(&source_description);
    if (source_description.Width != source_width ||
        source_description.Height != source_height ||
        source_description.Format != DXGI_FORMAT_B8G8R8A8_UNORM) {
      throw NvencError("NVENC received an unsupported GPU capture texture");
    }
    ComPtr<IDXGIResource> shared_resource;
    HANDLE shared_handle = nullptr;
    if (FAILED(source.QueryInterface(IID_PPV_ARGS(&shared_resource))) ||
        FAILED(shared_resource->GetSharedHandle(&shared_handle)) ||
        shared_handle == nullptr) {
      throw NvencError("The GPU capture texture is not shareable");
    }
    ComPtr<ID3D11Texture2D> input;
    if (FAILED(
            device_->OpenSharedResource(shared_handle, IID_PPV_ARGS(&input)))) {
      throw NvencError("NVENC could not open the GPU capture texture");
    }
    ComPtr<IDXGIKeyedMutex> keyed_mutex;
    if (FAILED(input.As(&keyed_mutex)) ||
        FAILED(keyed_mutex->AcquireSync(1, 5'000))) {
      throw NvencError("NVENC could not acquire the GPU capture texture");
    }
    try {
      ensure_video_processor(source_width, source_height);
      D3D11_VIDEO_PROCESSOR_INPUT_VIEW_DESC view{};
      view.FourCC = 0;
      view.ViewDimension = D3D11_VPIV_DIMENSION_TEXTURE2D;
      view.Texture2D.MipSlice = 0;
      view.Texture2D.ArraySlice = 0;
      ComPtr<ID3D11VideoProcessorInputView> input_view;
      if (FAILED(video_device_->CreateVideoProcessorInputView(
              input.Get(), video_enumerator_.Get(), &view, &input_view))) {
        throw NvencError("Could not create a BGRA video processor input");
      }
      const RECT source_rectangle{0, 0, static_cast<LONG>(source_width),
                                  static_cast<LONG>(source_height)};
      const RECT destination_rectangle{0, 0, static_cast<LONG>(width_),
                                       static_cast<LONG>(height_)};
      video_context_->VideoProcessorSetStreamSourceRect(
          video_processor_.Get(), 0, TRUE, &source_rectangle);
      video_context_->VideoProcessorSetStreamDestRect(
          video_processor_.Get(), 0, TRUE, &destination_rectangle);
      video_context_->VideoProcessorSetOutputTargetRect(
          video_processor_.Get(), TRUE, &destination_rectangle);
      if (video_context1_) {
        video_context1_->VideoProcessorSetStreamColorSpace1(
            video_processor_.Get(), 0, DXGI_COLOR_SPACE_RGB_FULL_G22_NONE_P709);
        video_context1_->VideoProcessorSetOutputColorSpace1(
            video_processor_.Get(),
            DXGI_COLOR_SPACE_YCBCR_STUDIO_G22_LEFT_P709);
      }
      D3D11_VIDEO_PROCESSOR_STREAM stream{};
      stream.Enable = TRUE;
      stream.pInputSurface = input_view.Get();
      if (FAILED(video_context_->VideoProcessorBlt(
              video_processor_.Get(), surface.output_view.Get(),
              static_cast<UINT>(encoded_frame_index_++), 1, &stream))) {
        throw NvencError("The GPU BGRA-to-NV12 conversion failed");
      }
    } catch (...) {
      static_cast<void>(keyed_mutex->ReleaseSync(0));
      throw;
    }
    if (FAILED(keyed_mutex->ReleaseSync(0))) {
      throw NvencError("NVENC could not release the GPU capture texture");
    }
  }

  void release() noexcept {
    if (encoder_ != nullptr) {
      for (auto &surface : surfaces_) {
        if (surface.bitstream != nullptr &&
            functions_.nvEncDestroyBitstreamBuffer != nullptr) {
          static_cast<void>(functions_.nvEncDestroyBitstreamBuffer(
              encoder_, surface.bitstream));
          surface.bitstream = nullptr;
        }
        if (surface.registered != nullptr &&
            functions_.nvEncUnregisterResource != nullptr) {
          static_cast<void>(
              functions_.nvEncUnregisterResource(encoder_, surface.registered));
          surface.registered = nullptr;
        }
        surface.output_view.Reset();
        surface.texture.Reset();
      }
      if (functions_.nvEncDestroyEncoder != nullptr) {
        static_cast<void>(functions_.nvEncDestroyEncoder(encoder_));
      }
      encoder_ = nullptr;
    }
  }

  std::uint32_t width_;
  std::uint32_t height_;
  std::uint32_t frames_per_second_;
  std::uint32_t target_bitrate_bps_;
  UniqueModule module_;
  NV_ENCODE_API_FUNCTION_LIST functions_{};
  ComPtr<IDXGIAdapter1> adapter_;
  ComPtr<ID3D11Device> device_;
  ComPtr<ID3D11DeviceContext> context_;
  ComPtr<ID3D11VideoDevice> video_device_;
  ComPtr<ID3D11VideoContext> video_context_;
  ComPtr<ID3D11VideoContext1> video_context1_;
  ComPtr<ID3D11VideoProcessorEnumerator> video_enumerator_;
  ComPtr<ID3D11VideoProcessor> video_processor_;
  void *encoder_ = nullptr;
  NV_ENC_CONFIG config_{};
  NV_ENC_INITIALIZE_PARAMS initialization_{};
  std::array<Surface, kSurfaceCount> surfaces_{};
  std::size_t surface_index_ = 0;
  std::uint32_t processor_source_width_ = 0;
  std::uint32_t processor_source_height_ = 0;
  std::uint64_t encoded_frame_index_ = 0;
  double last_gpu_conversion_submit_ms_ = 0;
  double last_encoder_submit_ms_ = 0;
  double last_bitstream_wait_ms_ = 0;
  bool finished_ = false;
  std::string implementation_name_ = "NVIDIA NVENC H.264 (direct D3D11)";
};

} // namespace

std::unique_ptr<H264Encoder>
create_hardware_h264_encoder(const std::uint32_t width,
                             const std::uint32_t height,
                             const std::uint32_t frames_per_second,
                             const std::uint32_t target_bitrate_bps) {
  try {
    return std::make_unique<DirectNvencH264Encoder>(
        width, height, frames_per_second, target_bitrate_bps);
  } catch (const std::exception &error) {
    std::cerr << "[Chatto Desktop capture] Direct NVENC unavailable; using "
                 "Media Foundation: "
              << error.what() << '\n';
    return std::make_unique<MediaFoundationH264Encoder>(
        width, height, frames_per_second, target_bitrate_bps);
  }
}

} // namespace chatto::capture
