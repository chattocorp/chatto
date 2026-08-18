// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#pragma once

#include <cstdint>
#include <memory>
#include <optional>
#include <span>
#include <string>
#include <vector>

#include <d3d11.h>

namespace chatto::capture {

struct EncodedH264AccessUnit {
  std::vector<std::uint8_t> data;
  std::int64_t timestamp_us = 0;
  bool key_frame = false;
};

struct H264ProfileLevel {
  std::uint8_t profile_idc = 0;
  std::uint8_t profile_iop = 0;
  std::uint8_t level_idc = 0;
};

/** Common interface for a realtime hardware H.264 encoder. */
class H264Encoder {
public:
  virtual ~H264Encoder() = default;

  [[nodiscard]] virtual std::vector<EncodedH264AccessUnit>
  encode(std::span<const std::uint8_t> bgra, std::int64_t timestamp_us,
         bool force_key_frame) = 0;
  /** Scale/convert a GPU BGRA texture and encode it without CPU pixel copies.
   */
  [[nodiscard]] virtual std::vector<EncodedH264AccessUnit>
  encode_gpu(ID3D11Texture2D &bgra_texture, std::uint32_t source_width,
             std::uint32_t source_height, std::int64_t timestamp_us,
             bool force_key_frame) = 0;
  virtual void set_target_bitrate(std::uint32_t target_bitrate_bps) = 0;
  [[nodiscard]] virtual std::uint32_t target_bitrate_bps() const noexcept = 0;
  [[nodiscard]] virtual std::uint32_t rate_control_mode() const noexcept = 0;
  [[nodiscard]] virtual std::vector<EncodedH264AccessUnit> finish() = 0;
  [[nodiscard]] virtual double last_gpu_conversion_submit_ms() const noexcept {
    return 0;
  }
  [[nodiscard]] virtual double last_encoder_submit_ms() const noexcept {
    return 0;
  }
  [[nodiscard]] virtual double last_bitstream_wait_ms() const noexcept {
    return 0;
  }
  [[nodiscard]] virtual const std::string &
  implementation_name() const noexcept = 0;
};

/** Convert a tightly packed, even-sized BGRA frame to BT.709 limited NV12. */
[[nodiscard]] std::vector<std::uint8_t>
bgra_to_nv12(std::span<const std::uint8_t> bgra, std::uint32_t width,
             std::uint32_t height);

/** Whether an Annex-B H.264 access unit contains an IDR slice. */
[[nodiscard]] bool
h264_access_unit_is_key_frame(std::span<const std::uint8_t> access_unit);

/** Whether an H.264 access unit begins with an Annex-B start code. */
[[nodiscard]] bool
h264_access_unit_is_annex_b(std::span<const std::uint8_t> access_unit);

/** Read profile-level-id bytes from the first Annex-B SPS, when present. */
[[nodiscard]] std::optional<H264ProfileLevel>
h264_access_unit_profile_level(std::span<const std::uint8_t> access_unit);

/** Low-latency hardware H.264 encoder backed by Media Foundation. */
class MediaFoundationH264Encoder final : public H264Encoder {
public:
  MediaFoundationH264Encoder(std::uint32_t width, std::uint32_t height,
                             std::uint32_t frames_per_second,
                             std::uint32_t target_bitrate_bps);
  ~MediaFoundationH264Encoder() override;

  MediaFoundationH264Encoder(const MediaFoundationH264Encoder &) = delete;
  MediaFoundationH264Encoder &
  operator=(const MediaFoundationH264Encoder &) = delete;

  /** Encode one BGRA frame and return any access units now available. */
  [[nodiscard]] std::vector<EncodedH264AccessUnit>
  encode(std::span<const std::uint8_t> bgra, std::int64_t timestamp_us,
         bool force_key_frame) override;

  [[nodiscard]] std::vector<EncodedH264AccessUnit>
  encode_gpu(ID3D11Texture2D &bgra_texture, std::uint32_t source_width,
             std::uint32_t source_height, std::int64_t timestamp_us,
             bool force_key_frame) override;

  /** Apply the latest WebRTC target bitrate when the encoder supports it. */
  void set_target_bitrate(std::uint32_t target_bitrate_bps) override;

  /** Latest bitrate value accepted and reported by the encoder. */
  [[nodiscard]] std::uint32_t target_bitrate_bps() const noexcept override;

  /** Active Media Foundation rate-control mode reported by the encoder. */
  [[nodiscard]] std::uint32_t rate_control_mode() const noexcept override;

  /** Drain delayed access units before shutting the publisher down. */
  [[nodiscard]] std::vector<EncodedH264AccessUnit> finish() override;

  /** Friendly name reported by the selected hardware Media Foundation MFT. */
  [[nodiscard]] const std::string &
  implementation_name() const noexcept override;

private:
  class Implementation;
  std::unique_ptr<Implementation> implementation_;
};

/** Prefer direct NVENC and fall back to the Windows Media Foundation MFT. */
[[nodiscard]] std::unique_ptr<H264Encoder>
create_hardware_h264_encoder(std::uint32_t width, std::uint32_t height,
                             std::uint32_t frames_per_second,
                             std::uint32_t target_bitrate_bps);

} // namespace chatto::capture
