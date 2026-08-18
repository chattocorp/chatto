// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#include "h264_encoder.h"

#include <cassert>
#include <cstdint>
#include <iostream>
#include <stdexcept>
#include <string_view>
#include <vector>

using chatto::capture::bgra_to_nv12;
using chatto::capture::h264_access_unit_is_annex_b;
using chatto::capture::h264_access_unit_is_key_frame;
using chatto::capture::h264_access_unit_profile_level;

int main(const int argument_count, char **arguments) {
  const std::vector<std::uint8_t> black_bgra(2 * 2 * 4, 0);
  const auto black_nv12 = bgra_to_nv12(black_bgra, 2, 2);
  assert(black_nv12.size() == 6);
  assert(black_nv12[0] == 16);
  assert(black_nv12[1] == 16);
  assert(black_nv12[2] == 16);
  assert(black_nv12[3] == 16);
  assert(black_nv12[4] == 128);
  assert(black_nv12[5] == 128);

  bool rejected_odd_dimensions = false;
  try {
    static_cast<void>(bgra_to_nv12(black_bgra, 1, 4));
  } catch (const std::invalid_argument &) {
    rejected_odd_dimensions = true;
  }
  assert(rejected_odd_dimensions);

  const std::vector<std::uint8_t> idr{
      0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0xe0,
      0x1f, 0x00, 0x00, 0x01, 0x65, 0xaa,
  };
  assert(h264_access_unit_is_key_frame(idr));
  assert(h264_access_unit_is_annex_b(idr));
  const auto profile_level = h264_access_unit_profile_level(idr);
  assert(profile_level.has_value());
  assert(profile_level->profile_idc == 0x42);
  assert(profile_level->profile_iop == 0xe0);
  assert(profile_level->level_idc == 0x1f);

  const std::vector<std::uint8_t> delta{
      0x00, 0x00, 0x00, 0x01, 0x41, 0xaa,
  };
  assert(!h264_access_unit_is_key_frame(delta));
  assert(h264_access_unit_is_annex_b(delta));

  if (argument_count > 1 && std::string_view(arguments[1]) == "--hardware") {
    try {
      constexpr std::uint32_t width = 1280;
      constexpr std::uint32_t height = 720;
      auto encoder = chatto::capture::create_hardware_h264_encoder(
          width, height, 60, 8'000'000);
      std::vector<std::uint8_t> frame(
          static_cast<std::size_t>(width) * height * 4, 0);
      std::size_t access_units = 0;
      std::size_t key_frames = 0;
      std::size_t annex_b_access_units = 0;
      std::optional<chatto::capture::H264ProfileLevel> hardware_profile_level;
      for (std::int64_t index = 0; index < 10; ++index) {
        if (index == 5) {
          encoder->set_target_bitrate(4'000'000);
          assert(encoder->target_bitrate_bps() == 4'000'000);
        }
        for (auto &access_unit :
             encoder->encode(frame, index * 1'000'000 / 60, index == 0)) {
          ++access_units;
          key_frames += access_unit.key_frame ? 1U : 0U;
          annex_b_access_units +=
              h264_access_unit_is_annex_b(access_unit.data) ? 1U : 0U;
          if (!hardware_profile_level.has_value()) {
            hardware_profile_level =
                h264_access_unit_profile_level(access_unit.data);
          }
        }
      }
      for (auto &access_unit : encoder->finish()) {
        ++access_units;
        key_frames += access_unit.key_frame ? 1U : 0U;
        annex_b_access_units +=
            h264_access_unit_is_annex_b(access_unit.data) ? 1U : 0U;
        if (!hardware_profile_level.has_value()) {
          hardware_profile_level =
              h264_access_unit_profile_level(access_unit.data);
        }
      }
      std::cout << "hardware_encoder=" << encoder->implementation_name()
                << " access_units=" << access_units
                << " key_frames=" << key_frames
                << " annex_b_access_units=" << annex_b_access_units;
      if (hardware_profile_level.has_value()) {
        std::cout << " profile_idc="
                  << static_cast<unsigned>(hardware_profile_level->profile_idc)
                  << " profile_iop="
                  << static_cast<unsigned>(hardware_profile_level->profile_iop)
                  << " level_idc="
                  << static_cast<unsigned>(hardware_profile_level->level_idc);
      }
      std::cout << '\n';
      assert(access_units > 0);
      assert(key_frames > 0);
      assert(annex_b_access_units == access_units);
    } catch (const std::exception &error) {
      std::cerr << "hardware encoder smoke test failed: " << error.what()
                << '\n';
      return 1;
    }
  }
  return 0;
}
