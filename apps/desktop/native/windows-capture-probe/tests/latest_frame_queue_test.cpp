// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#include <cstdint>
#include <cstdlib>
#include <iostream>
#include <vector>

#include "../src/latest_frame_queue.h"
#include "../src/video_frame_scaler.h"

namespace {

void expect(const bool condition, const char *message) {
  if (!condition) {
    std::cerr << "FAILED: " << message << "\n";
    std::exit(1);
  }
}

} // namespace

int main() {
  chatto::capture::LatestFrameQueue<int> queue;

  expect(!queue.push(1), "the first frame is accepted without a drop");
  expect(queue.push(2), "a pending frame is replaced");
  const auto latest = queue.wait_pop();
  expect(latest && *latest == 2, "the consumer receives only the latest frame");

  expect(!queue.push(3), "the empty queue accepts another frame");
  queue.close();
  const auto final = queue.wait_pop();
  expect(final && *final == 3, "close drains the final pending frame");
  expect(!queue.wait_pop(), "a closed drained queue finishes the consumer");
  expect(!queue.push(4), "a closed queue rejects later frames");

  std::vector<std::uint8_t> resized_source(4U * 2U * 4U);
  for (std::size_t pixel = 0; pixel < resized_source.size() / 4; ++pixel) {
    resized_source[pixel * 4] = static_cast<std::uint8_t>(pixel);
  }
  const auto resized =
      chatto::capture::scale_bgra_frame(std::move(resized_source), 4, 2, 2, 1);
  expect(resized.size() == 2U * 4U,
         "a resized source produces the stable track dimensions");
  expect(resized[0] == 0 && resized[4] == 2,
         "scaling samples across the complete resized source frame");

  std::cout << "latest frame queue tests passed\n";
  return 0;
}
