// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#pragma once

#include <condition_variable>
#include <mutex>
#include <optional>
#include <utility>

namespace chatto::capture {

// A single-slot handoff for realtime media. Producers replace stale work
// instead of allowing latency to accumulate behind a slow consumer.
template <typename Value> class LatestFrameQueue final {
public:
  LatestFrameQueue() = default;
  LatestFrameQueue(const LatestFrameQueue &) = delete;
  LatestFrameQueue &operator=(const LatestFrameQueue &) = delete;

  // Returns true when an older pending value was dropped.
  [[nodiscard]] bool push(Value value) {
    std::scoped_lock lock(mutex_);
    if (closed_) {
      return false;
    }
    const bool replaced = pending_.has_value();
    pending_ = std::move(value);
    changed_.notify_one();
    return replaced;
  }

  // Drains the final pending value after close, then returns nullopt.
  [[nodiscard]] std::optional<Value> wait_pop() {
    std::unique_lock lock(mutex_);
    changed_.wait(lock, [this] { return closed_ || pending_.has_value(); });
    if (!pending_) {
      return std::nullopt;
    }
    auto value = std::move(pending_);
    pending_.reset();
    return value;
  }

  void close() {
    std::scoped_lock lock(mutex_);
    closed_ = true;
    changed_.notify_all();
  }

private:
  std::mutex mutex_;
  std::condition_variable changed_;
  std::optional<Value> pending_;
  bool closed_ = false;
};

} // namespace chatto::capture
