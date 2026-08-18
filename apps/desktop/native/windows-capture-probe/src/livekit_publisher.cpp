// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

#include "livekit_publisher.h"

#include "audio_capture.h"
#include "h264_encoder.h"
#include "latest_frame_queue.h"
#include "video_capture.h"
#include "video_frame_scaler.h"
#include "window_sources.h"

#include <algorithm>
#include <atomic>
#include <chrono>
#include <cmath>
#include <condition_variable>
#include <exception>
#include <future>
#include <iomanip>
#include <iostream>
#include <limits>
#include <memory>
#include <mutex>
#include <optional>
#include <stop_token>
#include <thread>
#include <vector>

#include <livekit/audio_frame.h>
#include <livekit/audio_source.h>
#include <livekit/e2ee.h>
#include <livekit/encoded_video_source.h>
#include <livekit/livekit.h>
#include <livekit/local_audio_track.h>
#include <livekit/local_participant.h>
#include <livekit/local_video_track.h>
#include <livekit/room.h>
#include <livekit/room_event_types.h>
#include <livekit/stats.h>
#include <winrt/base.h>

namespace chatto::capture {
namespace {

class LiveKitRuntime final {
public:
  LiveKitRuntime() : initialized_here_(livekit::initialize()) {}

  ~LiveKitRuntime() {
    if (initialized_here_) {
      livekit::shutdown();
    }
  }

  LiveKitRuntime(const LiveKitRuntime &) = delete;
  LiveKitRuntime &operator=(const LiveKitRuntime &) = delete;

private:
  bool initialized_here_;
};

[[nodiscard]] std::int16_t float_to_pcm16(const float sample) {
  const float bounded = std::clamp(sample, -1.0F, 1.0F);
  return static_cast<std::int16_t>(std::lrint(
      bounded * static_cast<float>(std::numeric_limits<std::int16_t>::max())));
}

[[nodiscard]] std::pair<std::uint32_t, std::uint32_t>
bounded_size(const std::uint32_t width, const std::uint32_t height,
             const std::uint32_t maximum_width,
             const std::uint32_t maximum_height) {
  const double scale = std::min({
      1.0,
      static_cast<double>(maximum_width) / width,
      static_cast<double>(maximum_height) / height,
  });
  const auto even_dimension = [](const double value) {
    return std::max(2U, static_cast<std::uint32_t>(value) & ~1U);
  };
  return {even_dimension(width * scale), even_dimension(height * scale)};
}

struct VideoPumpMetrics {
  std::uint64_t submitted = 0;
  std::uint64_t published = 0;
  std::uint64_t dropped = 0;
};

struct RtcVideoMetrics {
  std::mutex mutex;
  bool available = false;
  std::uint32_t outbound_streams = 0;
  std::uint32_t active_outbound_streams = 0;
  double minimum_active_fps = 0;
  double maximum_active_fps = 0;
  std::uint64_t frames_encoded = 0;
  std::uint64_t frames_sent = 0;
  std::uint64_t bytes_sent = 0;
  std::uint64_t retransmitted_packets_sent = 0;
  std::uint64_t retransmitted_bytes_sent = 0;
  std::uint32_t nack_count = 0;
  std::uint32_t pli_count = 0;
  double target_bitrate = 0;
  double average_encode_ms = 0;
  std::uint32_t encoded_width = 0;
  std::uint32_t encoded_height = 0;
  double average_qp = 0;
  std::string encoder_implementation;
  std::uint32_t cpu_limited_streams = 0;
  std::uint32_t bandwidth_limited_streams = 0;
  std::uint32_t power_efficient_streams = 0;
  bool remote_inbound_available = false;
  std::int64_t remote_packets_lost = 0;
  double remote_jitter_seconds = 0;
  double remote_fraction_lost = 0;
  double remote_round_trip_time_ms = 0;
  bool candidate_pair_available = false;
  double available_outgoing_bitrate = 0;
  double current_round_trip_time_ms = 0;
  std::uint32_t packets_discarded_on_send = 0;
  std::uint64_t bytes_discarded_on_send = 0;
};

enum class CaptureBackend {
  WgcWindow,
  WgcMonitor,
  DxgiDisplay,
};

// Keep scaling, hardware encoding and the synchronous LiveKit FFI off the
// Windows Graphics Capture callback. Realtime capture replaces the one pending
// frame rather than accumulating latency when any downstream stage falls
// behind.
class LiveKitVideoPump final {
public:
  LiveKitVideoPump(std::shared_ptr<livekit::EncodedVideoSource> video_source,
                   std::shared_ptr<RtcVideoMetrics> rtc_metrics,
                   const std::uint32_t output_width,
                   const std::uint32_t output_height,
                   const std::uint32_t frames_per_second,
                   const std::uint32_t target_bitrate_bps,
                   EncodedPreviewCallback preview_callback)
      : video_source_(std::move(video_source)),
        rtc_metrics_(std::move(rtc_metrics)), output_width_(output_width),
        output_height_(output_height), frames_per_second_(frames_per_second),
        target_bitrate_bps_(target_bitrate_bps),
        preview_callback_(std::move(preview_callback)),
        requested_encoder_bitrate_bps_(target_bitrate_bps),
        requested_encoder_fps_(static_cast<double>(frames_per_second)),
        worker_([this] { run(); }),
        reporter_([this] { report(); }) {}

  ~LiveKitVideoPump() {
    queue_.close();
    if (worker_.joinable()) {
      worker_.join();
    }
    stop_reporter();
  }

  LiveKitVideoPump(const LiveKitVideoPump &) = delete;
  LiveKitVideoPump &operator=(const LiveKitVideoPump &) = delete;

  void submit(VideoFrameData frame) {
    rethrow_failure();
    submitted_.fetch_add(1, std::memory_order_relaxed);
    const auto dimensions =
        (static_cast<std::uint64_t>(frame.width) << 32U) | frame.height;
    const auto previous_dimensions =
        latest_dimensions_.exchange(dimensions, std::memory_order_relaxed);
    if (previous_dimensions != 0 && previous_dimensions != dimensions) {
      dimension_changes_.fetch_add(1, std::memory_order_relaxed);
    }
    readback_microseconds_.fetch_add(
        static_cast<std::uint64_t>(frame.readback_duration_ms * 1'000.0),
        std::memory_order_relaxed);
    if (queue_.push(std::move(frame))) {
      dropped_.fetch_add(1, std::memory_order_relaxed);
    }
  }

  void set_capture_backend(const CaptureBackend backend) {
    capture_backend_.store(backend, std::memory_order_relaxed);
  }

  [[nodiscard]] VideoPumpMetrics finish() {
    queue_.close();
    if (worker_.joinable()) {
      worker_.join();
    }
    stop_reporter();
    rethrow_failure();
    return {
        .submitted = submitted_.load(std::memory_order_relaxed),
        .published = published_.load(std::memory_order_relaxed),
        .dropped = dropped_.load(std::memory_order_relaxed),
    };
  }

private:
  void run() noexcept {
    try {
      winrt::init_apartment(winrt::apartment_type::multi_threaded);
      const auto encoder_width = output_width_;
      const auto encoder_height = output_height_;
      auto encoder = create_hardware_h264_encoder(
          output_width_, output_height_, frames_per_second_,
          target_bitrate_bps_);
      {
        std::scoped_lock lock(encoder_metrics_mutex_);
        hardware_encoder_implementation_ = encoder->implementation_name();
      }
      encoder_width_.store(encoder_width, std::memory_order_relaxed);
      encoder_height_.store(encoder_height, std::memory_order_relaxed);
      applied_encoder_bitrate_bps_.store(encoder->target_bitrate_bps(),
                                         std::memory_order_relaxed);
      encoder_rate_control_mode_.store(encoder->rate_control_mode(),
                                       std::memory_order_relaxed);
      bool keyframe_request_in_flight = false;
      while (auto frame = queue_.wait_pop()) {
        const std::int64_t timestamp_100ns = frame->timestamp_100ns;
        const auto feedback = video_source_->takeFeedback();
        if (feedback.rate_control &&
            feedback.rate_control->target_bitrate_bps > 0) {
          const auto target = static_cast<std::uint32_t>(std::min<std::uint64_t>(
              feedback.rate_control->target_bitrate_bps,
              std::numeric_limits<std::uint32_t>::max()));
          requested_encoder_bitrate_bps_.store(target,
                                               std::memory_order_relaxed);
          requested_encoder_fps_.store(feedback.rate_control->framerate_fps,
                                       std::memory_order_relaxed);
          encoder->set_target_bitrate(target);
          applied_encoder_bitrate_bps_.store(encoder->target_bitrate_bps(),
                                             std::memory_order_relaxed);
        }
        const auto scale_start = std::chrono::steady_clock::now();
        // WGC changes texture dimensions when a window enters or leaves
        // fullscreen. Scale every delivered frame from its actual dimensions
        // into the track's stable encoder size.
        auto scaled = scale_bgra_frame(std::move(frame->bgra), frame->width,
                                       frame->height, encoder_width,
                                       encoder_height);
        const auto scale_end = std::chrono::steady_clock::now();
        const auto encode_start = std::chrono::steady_clock::now();
        // WebRTC repeats its keyframe request until the requested IDR reaches
        // the RTP sender. An asynchronous MFT can have several frames in
        // flight, so forwarding every repetition would create a burst of IDRs
        // and waste much of the available bitrate.
        const bool force_key_frame =
            feedback.keyframe_requested && !keyframe_request_in_flight;
        keyframe_request_in_flight |= force_key_frame;
        auto access_units = encoder->encode(scaled, timestamp_100ns / 10,
                                            force_key_frame);
        if (std::any_of(access_units.begin(), access_units.end(),
                        [](const auto &access_unit) {
                          return access_unit.key_frame;
                        })) {
          keyframe_request_in_flight = false;
        }
        const auto encode_end = std::chrono::steady_clock::now();
        scale_microseconds_.fetch_add(
            static_cast<std::uint64_t>(
                std::chrono::duration<double, std::micro>(scale_end -
                                                          scale_start)
                    .count()),
            std::memory_order_relaxed);
        encode_microseconds_.fetch_add(
            static_cast<std::uint64_t>(
                std::chrono::duration<double, std::micro>(encode_end -
                                                          encode_start)
                    .count()),
            std::memory_order_relaxed);
        encoded_.fetch_add(1, std::memory_order_relaxed);
        publish_access_units(std::move(access_units), encoder_width,
                             encoder_height);
      }
      publish_access_units(encoder->finish(), encoder_width, encoder_height);
    } catch (...) {
      {
        std::scoped_lock lock(failure_mutex_);
        failure_ = std::current_exception();
      }
      queue_.close();
    }
  }

  void publish_access_units(std::vector<EncodedH264AccessUnit> access_units,
                            const std::uint32_t encoded_width,
                            const std::uint32_t encoded_height) {
    for (auto &access_unit : access_units) {
      hardware_encoded_frames_.fetch_add(1, std::memory_order_relaxed);
      hardware_encoded_bytes_.fetch_add(access_unit.data.size(),
                                        std::memory_order_relaxed);
      if (access_unit.key_frame) {
        hardware_key_frames_.fetch_add(1, std::memory_order_relaxed);
      }
      if (preview_callback_) {
        preview_callback_(access_unit.data, access_unit.timestamp_us,
                          access_unit.key_frame);
      }
      livekit::EncodedVideoFrame frame;
      frame.data = access_unit.data.data();
      frame.size = access_unit.data.size();
      frame.codec = livekit::EncodedVideoCodec::H264;
      frame.frame_type = access_unit.key_frame
                             ? livekit::EncodedVideoFrameType::Key
                             : livekit::EncodedVideoFrameType::Delta;
      frame.timestamp_us = access_unit.timestamp_us;
      frame.width = encoded_width;
      frame.height = encoded_height;
      const auto publish_start = std::chrono::steady_clock::now();
      const bool accepted = video_source_->captureFrame(frame);
      const auto publish_end = std::chrono::steady_clock::now();
      const auto publish_microseconds = static_cast<std::uint64_t>(
          std::chrono::duration<double, std::micro>(publish_end - publish_start)
              .count());
      publish_microseconds_.fetch_add(publish_microseconds,
                                      std::memory_order_relaxed);
      last_publish_microseconds_.store(publish_microseconds,
                                       std::memory_order_relaxed);
      if (accepted) {
        published_.fetch_add(1, std::memory_order_relaxed);
      } else {
        dropped_.fetch_add(1, std::memory_order_relaxed);
      }
    }
  }

  void report() noexcept {
    std::unique_lock lock(reporter_mutex_);
    auto previous_report_at = started_;
    std::uint64_t previous_hardware_encoded_bytes = 0;
    while (!reporter_changed_.wait_for(lock, std::chrono::seconds(2),
                                       [this] { return reporter_stopping_; })) {
      lock.unlock();
      const auto now = std::chrono::steady_clock::now();
      const auto hardware_encoded_bytes =
          hardware_encoded_bytes_.load(std::memory_order_relaxed);
      const double interval_seconds =
          std::chrono::duration<double>(now - previous_report_at).count();
      const double actual_hardware_bitrate =
          interval_seconds > 0
              ? static_cast<double>(hardware_encoded_bytes -
                                    previous_hardware_encoded_bytes) *
                    8.0 / interval_seconds
              : 0;
      emit_metrics(last_publish_microseconds_.load(std::memory_order_relaxed),
                   now, actual_hardware_bitrate);
      previous_report_at = now;
      previous_hardware_encoded_bytes = hardware_encoded_bytes;
      lock.lock();
    }
  }

  void stop_reporter() {
    {
      std::scoped_lock lock(reporter_mutex_);
      reporter_stopping_ = true;
    }
    reporter_changed_.notify_all();
    if (reporter_.joinable()) {
      reporter_.join();
    }
  }

  void emit_metrics(const std::uint64_t last_publish_microseconds,
                    const std::chrono::steady_clock::time_point now,
                    const double actual_hardware_bitrate) const {
    const auto submitted = submitted_.load(std::memory_order_relaxed);
    const auto published = published_.load(std::memory_order_relaxed);
    const double elapsed_seconds =
        std::chrono::duration<double>(now - started_).count();
    const double average_readback_ms =
        submitted == 0 ? 0
                       : static_cast<double>(readback_microseconds_.load(
                             std::memory_order_relaxed)) /
                             static_cast<double>(submitted) / 1'000.0;
    const double average_scale_ms =
        published == 0 ? 0
                       : static_cast<double>(scale_microseconds_.load(
                             std::memory_order_relaxed)) /
                             static_cast<double>(published) / 1'000.0;
    const double average_publish_ms =
        published == 0 ? 0
                       : static_cast<double>(publish_microseconds_.load(
                             std::memory_order_relaxed)) /
                             static_cast<double>(published) / 1'000.0;
    const auto encoded = encoded_.load(std::memory_order_relaxed);
    const double average_hardware_encode_ms =
        encoded == 0
            ? 0
            : static_cast<double>(
                  encode_microseconds_.load(std::memory_order_relaxed)) /
                  static_cast<double>(encoded) / 1'000.0;
    const auto dimensions = latest_dimensions_.load(std::memory_order_relaxed);
    std::string hardware_encoder;
    {
      std::scoped_lock encoder_lock(encoder_metrics_mutex_);
      hardware_encoder = hardware_encoder_implementation_;
    }
    std::scoped_lock rtc_lock(rtc_metrics_->mutex);
    std::cout << std::fixed << std::setprecision(3)
              << "{\"protocolVersion\":1,\"kind\":\"metrics\""
              << ",\"submittedFrames\":" << submitted
              << ",\"publishedFrames\":" << published << ",\"droppedFrames\":"
              << dropped_.load(std::memory_order_relaxed) << ",\"captureFps\":"
              << (elapsed_seconds > 0 ? submitted / elapsed_seconds : 0)
              << ",\"publishFps\":"
              << (elapsed_seconds > 0 ? published / elapsed_seconds : 0)
              << ",\"averageReadbackMs\":" << average_readback_ms
              << ",\"averageScaleMs\":" << average_scale_ms
              << ",\"averagePublishMs\":" << average_publish_ms
              << ",\"averageHardwareEncodeMs\":"
              << average_hardware_encode_ms
              << ",\"hardwareEncoderImplementation\":"
              << std::quoted(hardware_encoder)
              << ",\"requestedEncoderBitrate\":"
              << requested_encoder_bitrate_bps_.load(
                     std::memory_order_relaxed)
              << ",\"appliedEncoderBitrate\":"
              << applied_encoder_bitrate_bps_.load(std::memory_order_relaxed)
              << ",\"actualHardwareBitrate\":" << actual_hardware_bitrate
              << ",\"encoderRateControlMode\":"
              << encoder_rate_control_mode_.load(std::memory_order_relaxed)
              << ",\"requestedEncoderFps\":"
              << requested_encoder_fps_.load(std::memory_order_relaxed)
              << ",\"hardwareEncodedFrames\":"
              << hardware_encoded_frames_.load(std::memory_order_relaxed)
              << ",\"hardwareEncodedBytes\":"
              << hardware_encoded_bytes_.load(std::memory_order_relaxed)
              << ",\"hardwareKeyFrames\":"
              << hardware_key_frames_.load(std::memory_order_relaxed)
              << ",\"hardwareEncodedWidth\":"
              << encoder_width_.load(std::memory_order_relaxed)
              << ",\"hardwareEncodedHeight\":"
              << encoder_height_.load(std::memory_order_relaxed)
              << ",\"encoderResolutionChanges\":"
              << encoder_resolution_changes_.load(std::memory_order_relaxed)
              << ",\"lastPublishMs\":"
              << static_cast<double>(last_publish_microseconds) / 1'000.0
              << ",\"sourceWidth\":" << (dimensions >> 32U)
              << ",\"sourceHeight\":"
              << (dimensions & std::numeric_limits<std::uint32_t>::max())
              << ",\"dimensionChanges\":"
              << dimension_changes_.load(std::memory_order_relaxed)
              << ",\"captureBackend\":\""
              << capture_backend_name(
                     capture_backend_.load(std::memory_order_relaxed))
              << "\""
              << ",\"rtcStatsAvailable\":"
              << (rtc_metrics_->available ? "true" : "false")
              << ",\"outboundStreams\":" << rtc_metrics_->outbound_streams
              << ",\"activeOutboundStreams\":"
              << rtc_metrics_->active_outbound_streams
              << ",\"minimumActiveOutboundFps\":"
              << rtc_metrics_->minimum_active_fps
              << ",\"maximumActiveOutboundFps\":"
              << rtc_metrics_->maximum_active_fps
              << ",\"framesEncoded\":" << rtc_metrics_->frames_encoded
              << ",\"framesSent\":" << rtc_metrics_->frames_sent
              << ",\"bytesSent\":" << rtc_metrics_->bytes_sent
              << ",\"retransmittedPacketsSent\":"
              << rtc_metrics_->retransmitted_packets_sent
              << ",\"retransmittedBytesSent\":"
              << rtc_metrics_->retransmitted_bytes_sent
              << ",\"nackCount\":" << rtc_metrics_->nack_count
              << ",\"pliCount\":" << rtc_metrics_->pli_count
              << ",\"targetBitrate\":" << rtc_metrics_->target_bitrate
              << ",\"averageEncodeMs\":" << rtc_metrics_->average_encode_ms
              << ",\"encodedWidth\":" << rtc_metrics_->encoded_width
              << ",\"encodedHeight\":" << rtc_metrics_->encoded_height
              << ",\"averageQp\":" << rtc_metrics_->average_qp
              << ",\"encoderImplementation\":"
              << std::quoted(rtc_metrics_->encoder_implementation)
              << ",\"cpuLimitedStreams\":" << rtc_metrics_->cpu_limited_streams
              << ",\"bandwidthLimitedStreams\":"
              << rtc_metrics_->bandwidth_limited_streams
              << ",\"powerEfficientStreams\":"
              << rtc_metrics_->power_efficient_streams
              << ",\"remoteInboundStatsAvailable\":"
              << (rtc_metrics_->remote_inbound_available ? "true" : "false")
              << ",\"remotePacketsLost\":"
              << rtc_metrics_->remote_packets_lost
              << ",\"remoteJitterSeconds\":"
              << rtc_metrics_->remote_jitter_seconds
              << ",\"remoteFractionLost\":"
              << rtc_metrics_->remote_fraction_lost
              << ",\"remoteRoundTripTimeMs\":"
              << rtc_metrics_->remote_round_trip_time_ms
              << ",\"candidatePairStatsAvailable\":"
              << (rtc_metrics_->candidate_pair_available ? "true" : "false")
              << ",\"availableOutgoingBitrate\":"
              << rtc_metrics_->available_outgoing_bitrate
              << ",\"currentRoundTripTimeMs\":"
              << rtc_metrics_->current_round_trip_time_ms
              << ",\"packetsDiscardedOnSend\":"
              << rtc_metrics_->packets_discarded_on_send
              << ",\"bytesDiscardedOnSend\":"
              << rtc_metrics_->bytes_discarded_on_send << "}\n"
              << std::flush;
  }

  void rethrow_failure() {
    std::scoped_lock lock(failure_mutex_);
    if (failure_) {
      std::rethrow_exception(failure_);
    }
  }

  [[nodiscard]] static const char *
  capture_backend_name(const CaptureBackend backend) {
    switch (backend) {
    case CaptureBackend::WgcWindow:
      return "wgc-window";
    case CaptureBackend::WgcMonitor:
      return "wgc-monitor";
    case CaptureBackend::DxgiDisplay:
      return "dxgi-display";
    }
    return "unknown";
  }

  std::shared_ptr<livekit::EncodedVideoSource> video_source_;
  std::shared_ptr<RtcVideoMetrics> rtc_metrics_;
  EncodedPreviewCallback preview_callback_;
  std::uint32_t output_width_;
  std::uint32_t output_height_;
  std::uint32_t frames_per_second_;
  std::uint32_t target_bitrate_bps_;
  LatestFrameQueue<VideoFrameData> queue_;
  std::mutex failure_mutex_;
  std::exception_ptr failure_;
  std::atomic<std::uint64_t> submitted_{0};
  std::atomic<std::uint64_t> published_{0};
  std::atomic<std::uint64_t> dropped_{0};
  std::atomic<std::uint64_t> readback_microseconds_{0};
  std::atomic<std::uint64_t> scale_microseconds_{0};
  std::atomic<std::uint64_t> publish_microseconds_{0};
  std::atomic<std::uint64_t> encode_microseconds_{0};
  std::atomic<std::uint64_t> encoded_{0};
  std::atomic<std::uint32_t> requested_encoder_bitrate_bps_{0};
  std::atomic<std::uint32_t> applied_encoder_bitrate_bps_{0};
  std::atomic<std::uint32_t> encoder_rate_control_mode_{
      std::numeric_limits<std::uint32_t>::max()};
  std::atomic<double> requested_encoder_fps_{0};
  std::atomic<std::uint64_t> hardware_encoded_frames_{0};
  std::atomic<std::uint64_t> hardware_encoded_bytes_{0};
  std::atomic<std::uint64_t> hardware_key_frames_{0};
  std::atomic<std::uint32_t> encoder_width_{0};
  std::atomic<std::uint32_t> encoder_height_{0};
  std::atomic<std::uint64_t> encoder_resolution_changes_{0};
  std::atomic<std::uint64_t> last_publish_microseconds_{0};
  std::atomic<std::uint64_t> latest_dimensions_{0};
  std::atomic<std::uint64_t> dimension_changes_{0};
  std::atomic<CaptureBackend> capture_backend_{CaptureBackend::WgcWindow};
  std::chrono::steady_clock::time_point started_ =
      std::chrono::steady_clock::now();
  std::thread worker_;
  std::mutex reporter_mutex_;
  std::condition_variable reporter_changed_;
  bool reporter_stopping_ = false;
  std::thread reporter_;
  mutable std::mutex encoder_metrics_mutex_;
  std::string hardware_encoder_implementation_;
};

class LiveKitVideoStatsReporter final {
public:
  LiveKitVideoStatsReporter(
      std::shared_ptr<livekit::LocalVideoTrack> video_track,
      std::shared_ptr<RtcVideoMetrics> metrics)
      : video_track_(std::move(video_track)), metrics_(std::move(metrics)),
        worker_([this] { run(); }) {}

  ~LiveKitVideoStatsReporter() {
    {
      std::scoped_lock lock(stop_mutex_);
      stopping_ = true;
    }
    stop_changed_.notify_all();
    if (worker_.joinable()) {
      worker_.join();
    }
  }

  LiveKitVideoStatsReporter(const LiveKitVideoStatsReporter &) = delete;
  LiveKitVideoStatsReporter &
  operator=(const LiveKitVideoStatsReporter &) = delete;

private:
  void run() noexcept {
    std::unique_lock stop_lock(stop_mutex_);
    while (!stop_changed_.wait_for(stop_lock, std::chrono::seconds(2),
                                   [this] { return stopping_; })) {
      stop_lock.unlock();
      try {
        auto pending = video_track_->getStats();
        if (pending.wait_for(std::chrono::seconds(1)) ==
            std::future_status::ready) {
          update(pending.get());
        }
      } catch (...) {
        // Diagnostics are best-effort and must never stop publication.
      }
      stop_lock.lock();
    }
  }

  void update(const std::vector<livekit::RtcStats> &stats) {
    RtcVideoMetrics next;
    std::uint32_t encoded_streams = 0;
    std::uint64_t encoded_frames = 0;
    std::uint64_t qp_sum = 0;
    for (const auto &stat : stats) {
      const auto *outbound =
          std::get_if<livekit::RtcOutboundRtpStats>(&stat.stats);
      if (!outbound || outbound->stream.kind != "video") {
        if (const auto *remote =
                std::get_if<livekit::RtcRemoteInboundRtpStats>(&stat.stats);
            remote && remote->stream.kind == "video") {
          next.remote_inbound_available = true;
          next.remote_packets_lost += remote->received.packets_lost;
          next.remote_jitter_seconds =
              std::max(next.remote_jitter_seconds, remote->received.jitter);
          next.remote_fraction_lost = std::max(
              next.remote_fraction_lost, remote->remote_inbound.fraction_lost);
          next.remote_round_trip_time_ms =
              std::max(next.remote_round_trip_time_ms,
                       remote->remote_inbound.round_trip_time * 1'000.0);
        } else if (const auto *pair =
                       std::get_if<livekit::RtcCandidatePairStats>(&stat.stats);
                   pair && pair->candidate_pair.nominated &&
                   pair->candidate_pair.state ==
                       livekit::IceCandidatePairState::Succeeded) {
          next.candidate_pair_available = true;
          next.available_outgoing_bitrate = std::max(
              next.available_outgoing_bitrate,
              pair->candidate_pair.available_outgoing_bitrate);
          next.current_round_trip_time_ms = std::max(
              next.current_round_trip_time_ms,
              pair->candidate_pair.current_round_trip_time * 1'000.0);
          next.packets_discarded_on_send +=
              pair->candidate_pair.packets_discarded_on_send;
          next.bytes_discarded_on_send +=
              pair->candidate_pair.bytes_discarded_on_send;
        }
        continue;
      }
      next.outbound_streams += 1;
      next.frames_encoded += outbound->outbound.frames_encoded;
      next.frames_sent += outbound->outbound.frames_sent;
      next.bytes_sent += outbound->sent.bytes_sent;
      next.retransmitted_packets_sent +=
          outbound->outbound.retransmitted_packets_sent;
      next.retransmitted_bytes_sent +=
          outbound->outbound.retransmitted_bytes_sent;
      next.nack_count += outbound->outbound.nack_count;
      next.pli_count += outbound->outbound.pli_count;
      next.target_bitrate += outbound->outbound.target_bitrate;
      next.encoded_width =
          std::max(next.encoded_width, outbound->outbound.frame_width);
      next.encoded_height =
          std::max(next.encoded_height, outbound->outbound.frame_height);
      encoded_frames += outbound->outbound.frames_encoded;
      qp_sum += outbound->outbound.qp_sum;
      if (next.encoder_implementation.empty()) {
        next.encoder_implementation = outbound->outbound.encoder_implementation;
      }
      if (outbound->outbound.power_efficient_encoder) {
        next.power_efficient_streams += 1;
      }
      if (outbound->outbound.quality_limitation_reason ==
          livekit::QualityLimitationReason::Cpu) {
        next.cpu_limited_streams += 1;
      } else if (outbound->outbound.quality_limitation_reason ==
                 livekit::QualityLimitationReason::Bandwidth) {
        next.bandwidth_limited_streams += 1;
      }
      if (outbound->outbound.active) {
        next.active_outbound_streams += 1;
        const double fps = outbound->outbound.frames_per_second;
        if (next.active_outbound_streams == 1) {
          next.minimum_active_fps = fps;
        } else {
          next.minimum_active_fps = std::min(next.minimum_active_fps, fps);
        }
        next.maximum_active_fps = std::max(next.maximum_active_fps, fps);
      }
      if (outbound->outbound.frames_encoded > 0) {
        encoded_streams += 1;
        next.average_encode_ms +=
            outbound->outbound.total_encode_time /
            static_cast<double>(outbound->outbound.frames_encoded) * 1'000.0;
      }
    }
    if (next.outbound_streams > 0) {
      next.available = true;
    }
    if (encoded_streams > 0) {
      next.average_encode_ms /= encoded_streams;
    }
    if (encoded_frames > 0) {
      next.average_qp =
          static_cast<double>(qp_sum) / static_cast<double>(encoded_frames);
    }
    std::scoped_lock lock(metrics_->mutex);
    metrics_->available = next.available;
    metrics_->outbound_streams = next.outbound_streams;
    metrics_->active_outbound_streams = next.active_outbound_streams;
    metrics_->minimum_active_fps = next.minimum_active_fps;
    metrics_->maximum_active_fps = next.maximum_active_fps;
    metrics_->frames_encoded = next.frames_encoded;
    metrics_->frames_sent = next.frames_sent;
    metrics_->bytes_sent = next.bytes_sent;
    metrics_->retransmitted_packets_sent = next.retransmitted_packets_sent;
    metrics_->retransmitted_bytes_sent = next.retransmitted_bytes_sent;
    metrics_->nack_count = next.nack_count;
    metrics_->pli_count = next.pli_count;
    metrics_->target_bitrate = next.target_bitrate;
    metrics_->average_encode_ms = next.average_encode_ms;
    metrics_->encoded_width = next.encoded_width;
    metrics_->encoded_height = next.encoded_height;
    metrics_->average_qp = next.average_qp;
    metrics_->encoder_implementation = std::move(next.encoder_implementation);
    metrics_->cpu_limited_streams = next.cpu_limited_streams;
    metrics_->bandwidth_limited_streams = next.bandwidth_limited_streams;
    metrics_->power_efficient_streams = next.power_efficient_streams;
    metrics_->remote_inbound_available = next.remote_inbound_available;
    metrics_->remote_packets_lost = next.remote_packets_lost;
    metrics_->remote_jitter_seconds = next.remote_jitter_seconds;
    metrics_->remote_fraction_lost = next.remote_fraction_lost;
    metrics_->remote_round_trip_time_ms = next.remote_round_trip_time_ms;
    metrics_->candidate_pair_available = next.candidate_pair_available;
    metrics_->available_outgoing_bitrate = next.available_outgoing_bitrate;
    metrics_->current_round_trip_time_ms = next.current_round_trip_time_ms;
    metrics_->packets_discarded_on_send = next.packets_discarded_on_send;
    metrics_->bytes_discarded_on_send = next.bytes_discarded_on_send;
  }

  std::shared_ptr<livekit::LocalVideoTrack> video_track_;
  std::shared_ptr<RtcVideoMetrics> metrics_;
  std::mutex stop_mutex_;
  std::condition_variable stop_changed_;
  bool stopping_ = false;
  std::thread worker_;
};

constexpr auto kFrameStallTimeout = std::chrono::seconds(2);
constexpr auto kReplacementWindowTimeout = std::chrono::seconds(3);
constexpr std::uint32_t kInitialVideoBitrateBps = 12'000'000;

[[nodiscard]] std::optional<HWND> wait_for_replacement_window(
    HWND stale_window, const std::wstring &expected_application_identifier,
    const DWORD preferred_process_id, const bool allow_stale_window,
    const std::stop_token stop_token) {
  const auto deadline =
      std::chrono::steady_clock::now() + kReplacementWindowTimeout;
  do {
    const auto replacement = select_replacement_window_source(
        enumerate_window_sources(), expected_application_identifier,
        preferred_process_id, stale_window);
    if (replacement) {
      return replacement->handle;
    }
    if (allow_stale_window && is_window_capture_candidate(stale_window) &&
        window_matches_application(stale_window,
                                   expected_application_identifier)) {
      return stale_window;
    }
    std::this_thread::sleep_for(std::chrono::milliseconds(100));
  } while (!stop_token.stop_requested() &&
           std::chrono::steady_clock::now() < deadline);
  return std::nullopt;
}

} // namespace

int publish_window(HWND window,
                   const std::wstring &expected_application_identifier,
                   const std::uint32_t frames_per_second,
                   const std::uint32_t maximum_width,
                   const std::uint32_t maximum_height,
                   const PublisherCredential &credential,
                   EncodedPreviewCallback preview_callback,
                   const std::stop_token stop_token) {
  if (!window_matches_application(window, expected_application_identifier)) {
    throw std::invalid_argument(
        "The selected window no longer belongs to the offered application");
  }
  if (credential.livekit_url.empty() || credential.token.empty() ||
      credential.e2ee_key.empty()) {
    throw std::invalid_argument("The publisher credential is incomplete");
  }

  // LiveKit requires process-global initialization before constructing any
  // source, track, or room. Declare the guard first so shutdown runs only
  // after every SDK-backed object below has been destroyed.
  LiveKitRuntime livekit_runtime;
  const auto [source_width, source_height] = window_capture_size(window);
  const auto [width, height] =
      bounded_size(source_width, source_height, maximum_width, maximum_height);
  auto video_source =
      std::make_shared<livekit::EncodedVideoSource>(width, height);
  auto audio_source = std::make_shared<livekit::AudioSource>(48'000, 2, 0);
  auto video_track = livekit::LocalVideoTrack::createLocalVideoTrack(
      "game-capture-video", video_source);
  auto audio_track = livekit::LocalAudioTrack::createLocalAudioTrack(
      "game-capture-audio", audio_source);
  auto rtc_metrics = std::make_shared<RtcVideoMetrics>();
  LiveKitVideoPump video_pump(video_source, rtc_metrics, width, height,
                              frames_per_second, kInitialVideoBitrateBps,
                              std::move(preview_callback));

  livekit::RoomOptions room_options;
  room_options.auto_subscribe = false;
  // This publisher has one video layer and must outlive Desktop's local
  // preview subscription. With dynacast enabled, covering or minimizing the
  // Electron window can make its adaptive receiver stop consuming the track;
  // LiveKit then pauses the helper's only layer even though capture continues.
  room_options.dynacast = false;
  livekit::E2EEOptions encryption;
  encryption.key_provider_options.shared_key = std::vector<std::uint8_t>(
      credential.e2ee_key.begin(), credential.e2ee_key.end());
  room_options.encryption = std::move(encryption);

  livekit::Room room;
  if (!room.connect(credential.livekit_url, credential.token, room_options)) {
    throw std::runtime_error(
        "The native publisher could not connect to LiveKit");
  }
  if (const auto manager = room.e2eeManager().lock()) {
    manager->setEnabled(true);
  } else {
    throw std::runtime_error("The native publisher could not enable E2EE");
  }
  const auto participant = room.localParticipant().lock();
  if (!participant) {
    throw std::runtime_error("The native publisher has no local participant");
  }

  livekit::TrackPublishOptions video_options;
  video_options.video_encoding = livekit::VideoEncodingOptions{
      .max_bitrate = kInitialVideoBitrateBps,
      .max_framerate = static_cast<double>(frames_per_second),
  };
  video_options.video_codec = livekit::VideoCodec::H264;
  video_options.video_encoder = livekit::VideoEncoderBackend::PreEncoded;
  // LiveKit C++ 1.7 only exposes a simulcast switch, not custom screen-share
  // layers. Its default lowest screen-share layer is capped at 3 fps, which
  // adaptive-stream receivers select for Chatto's compact call tile. Publish
  // one full-cadence layer until the SDK lets us define a game-oriented ladder.
  video_options.simulcast = false;
  video_options.source = livekit::TrackSource::SOURCE_SCREENSHARE;
  video_options.stream = "game-capture";
  video_options.degradation_preference =
      livekit::DegradationPreference::MaintainFramerate;
  participant->publishTrack(video_track, video_options);

  livekit::TrackPublishOptions audio_options;
  audio_options.audio_encoding = livekit::AudioEncodingOptions{
      .max_bitrate = 128'000,
  };
  audio_options.dtx = false;
  // Companion metadata identifies this as isolated application audio. Use the
  // microphone wire source allowed by the existing companion credential, as
  // the macOS publisher does, rather than requiring a newer server grant.
  audio_options.source = livekit::TrackSource::SOURCE_MICROPHONE;
  audio_options.stream = "game-capture";
  participant->publishTrack(audio_track, audio_options);

  std::cout << "{\"protocolVersion\":1,\"kind\":\"started\",\"width\":" << width
            << ",\"height\":" << height
            << ",\"frameRate\":" << frames_per_second << "}\n"
            << std::flush;
  auto stats_reporter =
      std::make_unique<LiveKitVideoStatsReporter>(video_track, rtc_metrics);

  DWORD process_id = 0;
  GetWindowThreadProcessId(window, &process_id);
  std::stop_source audio_stop;
  auto audio_future =
      std::async(std::launch::async, [process_id, audio_source,
                                      stop_token = audio_stop.get_token()] {
        winrt::init_apartment(winrt::apartment_type::multi_threaded);
        return capture_process_audio(
            process_id, std::chrono::hours(24), stop_token, {},
            [audio_source](const AudioFrameData &frame) {
              auto livekit_frame = livekit::AudioFrame::create(
                  static_cast<int>(frame.sample_rate),
                  static_cast<int>(frame.channels),
                  static_cast<int>(frame.frames));
              if (!frame.silent) {
                auto &output = livekit_frame.data();
                for (std::size_t index = 0; index < output.size(); ++index) {
                  output[index] = float_to_pcm16(frame.samples[index]);
                }
              }
              audio_source->captureFrame(livekit_frame);
            });
      });

  try {
    auto capture_window = window;
    std::uint64_t captured_frames = 0;
    auto display_fallback_retry_at = std::chrono::steady_clock::time_point{};
    auto capture_backend = CaptureBackend::WgcWindow;
    while (!stop_token.stop_requested()) {
      const auto now = std::chrono::steady_clock::now();
      const bool monitor_covering =
          is_foreground_monitor_covering_window(capture_window);
      if (!monitor_covering) {
        capture_backend = CaptureBackend::WgcWindow;
      } else if (capture_backend == CaptureBackend::WgcWindow) {
        capture_backend = CaptureBackend::WgcMonitor;
      } else if (capture_backend == CaptureBackend::DxgiDisplay &&
                 now < display_fallback_retry_at) {
        capture_backend = CaptureBackend::WgcWindow;
      }
      video_pump.set_capture_backend(capture_backend);
      const auto submit_frame = [&video_pump](VideoFrameData frame) {
        video_pump.submit(std::move(frame));
      };
      VideoCaptureMetrics video_metrics;
      switch (capture_backend) {
      case CaptureBackend::WgcWindow:
        video_metrics = capture_window_video(
            capture_window, std::chrono::hours(24), frames_per_second, false,
            {}, submit_frame, stop_token, kFrameStallTimeout, true);
        break;
      case CaptureBackend::WgcMonitor:
        video_metrics = capture_monitor_covering_window_wgc_video(
            capture_window, std::chrono::hours(24), frames_per_second,
            submit_frame, stop_token, kFrameStallTimeout);
        break;
      case CaptureBackend::DxgiDisplay:
        video_metrics = capture_monitor_covering_window_dxgi_video(
            capture_window, std::chrono::hours(24), frames_per_second,
            submit_frame, stop_token);
        break;
      }
      captured_frames += video_metrics.frames;
      if (video_metrics.source_closed || video_metrics.frame_stalled ||
          video_metrics.presentation_changed || video_metrics.stop_requested) {
        std::cerr << "[Chatto Desktop capture] Window capture returned: "
                  << "backend=" << static_cast<int>(capture_backend)
                  << " frames=" << video_metrics.frames
                  << " sourceClosed=" << video_metrics.source_closed
                  << " frameStalled=" << video_metrics.frame_stalled
                  << " presentationChanged="
                  << video_metrics.presentation_changed
                  << " stopRequested=" << video_metrics.stop_requested
                  << " windowValid=" << IsWindow(capture_window) << "\n";
      }
      if (!video_metrics.error.empty()) {
        if (capture_backend != CaptureBackend::WgcWindow) {
          std::cerr << "[Chatto Desktop capture] Display fallback failed: "
                    << winrt::to_string(winrt::hstring(video_metrics.error))
                    << " (0x" << std::hex
                    << static_cast<std::uint32_t>(video_metrics.error_code)
                    << std::dec << ")\n";
          video_pump.set_capture_backend(CaptureBackend::WgcWindow);
          display_fallback_retry_at =
              std::chrono::steady_clock::now() + std::chrono::seconds(10);
          capture_backend = CaptureBackend::WgcWindow;
          continue;
        }
        throw std::runtime_error(
            winrt::to_string(winrt::hstring(video_metrics.error)));
      }
      if (video_metrics.stop_requested || stop_token.stop_requested()) {
        break;
      }
      if (video_metrics.presentation_changed) {
        std::cerr << "Windows video publisher returned to window capture\n";
        capture_backend = CaptureBackend::WgcWindow;
        continue;
      }
      if (!video_metrics.source_closed && !video_metrics.frame_stalled) {
        break;
      }

      const bool currently_monitor_covering =
          is_foreground_monitor_covering_window(capture_window);
      if (video_metrics.frame_stalled && currently_monitor_covering) {
        if (capture_backend == CaptureBackend::WgcWindow) {
          capture_backend = CaptureBackend::WgcMonitor;
          continue;
        }
        if (capture_backend == CaptureBackend::WgcMonitor) {
          capture_backend = CaptureBackend::DxgiDisplay;
          continue;
        }
      }

      const auto replacement = wait_for_replacement_window(
          capture_window, expected_application_identifier, process_id,
          video_metrics.frame_stalled || video_metrics.source_closed,
          stop_token);
      if (!replacement) {
        std::cerr << "[Chatto Desktop capture] Window capture ended because "
                     "no matching replacement window remained\n";
        break;
      }
      capture_window = *replacement;
      if (capture_backend == CaptureBackend::DxgiDisplay) {
        std::cerr << "Windows video publisher restarted DXGI display capture\n";
      } else {
        std::cerr << "Windows video publisher reattached after "
                  << (video_metrics.source_closed ? "source closure"
                                                  : "frame stall")
                  << "\n";
      }
    }
    const auto pump_metrics = video_pump.finish();
    std::cerr << "[Chatto Desktop capture] Windows video publisher ended: "
              << "stopRequested=" << stop_token.stop_requested()
              << " captured=" << captured_frames
              << " submitted=" << pump_metrics.submitted
              << " published=" << pump_metrics.published
              << " dropped=" << pump_metrics.dropped << "\n";
  } catch (...) {
    stats_reporter.reset();
    audio_stop.request_stop();
    if (audio_future.valid()) {
      try {
        static_cast<void>(audio_future.get());
      } catch (...) {
      }
    }
    room.disconnect();
    throw;
  }
  audio_stop.request_stop();
  if (audio_future.valid()) {
    static_cast<void>(audio_future.get());
  }
  stats_reporter.reset();
  room.disconnect();
  return 0;
}

int publish_display(HMONITOR monitor, const std::uint32_t frames_per_second,
                    const std::uint32_t maximum_width,
                    const std::uint32_t maximum_height,
                    const PublisherCredential &credential,
                    EncodedPreviewCallback preview_callback,
                    const std::stop_token stop_token) {
  if (!is_display_capture_candidate(monitor)) {
    throw std::invalid_argument("The selected monitor no longer exists");
  }
  if (credential.livekit_url.empty() || credential.token.empty() ||
      credential.e2ee_key.empty()) {
    throw std::invalid_argument("The publisher credential is incomplete");
  }

  MONITORINFO monitor_information{};
  monitor_information.cbSize = sizeof(monitor_information);
  winrt::check_bool(GetMonitorInfoW(monitor, &monitor_information));
  const auto source_width = static_cast<std::uint32_t>(
      monitor_information.rcMonitor.right - monitor_information.rcMonitor.left);
  const auto source_height = static_cast<std::uint32_t>(
      monitor_information.rcMonitor.bottom - monitor_information.rcMonitor.top);
  const auto [width, height] =
      bounded_size(source_width, source_height, maximum_width, maximum_height);

  LiveKitRuntime livekit_runtime;
  auto video_source =
      std::make_shared<livekit::EncodedVideoSource>(width, height);
  auto video_track = livekit::LocalVideoTrack::createLocalVideoTrack(
      "display-capture-video", video_source);
  auto rtc_metrics = std::make_shared<RtcVideoMetrics>();
  LiveKitVideoPump video_pump(video_source, rtc_metrics, width, height,
                              frames_per_second, kInitialVideoBitrateBps,
                              std::move(preview_callback));

  livekit::RoomOptions room_options;
  room_options.auto_subscribe = false;
  room_options.dynacast = false;
  livekit::E2EEOptions encryption;
  encryption.key_provider_options.shared_key = std::vector<std::uint8_t>(
      credential.e2ee_key.begin(), credential.e2ee_key.end());
  room_options.encryption = std::move(encryption);

  livekit::Room room;
  if (!room.connect(credential.livekit_url, credential.token, room_options)) {
    throw std::runtime_error(
        "The native publisher could not connect to LiveKit");
  }
  if (const auto manager = room.e2eeManager().lock()) {
    manager->setEnabled(true);
  } else {
    throw std::runtime_error("The native publisher could not enable E2EE");
  }
  const auto participant = room.localParticipant().lock();
  if (!participant) {
    throw std::runtime_error("The native publisher has no local participant");
  }

  livekit::TrackPublishOptions video_options;
  video_options.video_encoding = livekit::VideoEncodingOptions{
      .max_bitrate = kInitialVideoBitrateBps,
      .max_framerate = static_cast<double>(frames_per_second),
  };
  video_options.video_codec = livekit::VideoCodec::H264;
  video_options.video_encoder = livekit::VideoEncoderBackend::PreEncoded;
  video_options.simulcast = false;
  video_options.source = livekit::TrackSource::SOURCE_SCREENSHARE;
  video_options.stream = "game-capture";
  video_options.degradation_preference =
      livekit::DegradationPreference::MaintainFramerate;
  participant->publishTrack(video_track, video_options);

  std::cout << "{\"protocolVersion\":1,\"kind\":\"started\",\"width\":"
            << width << ",\"height\":" << height
            << ",\"frameRate\":" << frames_per_second << "}\n"
            << std::flush;
  auto stats_reporter =
      std::make_unique<LiveKitVideoStatsReporter>(video_track, rtc_metrics);
  video_pump.set_capture_backend(CaptureBackend::WgcMonitor);

  try {
    std::uint64_t captured_frames = 0;
    while (!stop_token.stop_requested() &&
           is_display_capture_candidate(monitor)) {
      const auto video_metrics = capture_monitor_wgc_video(
          monitor, std::chrono::hours(24), frames_per_second,
          [&video_pump](VideoFrameData frame) {
            video_pump.submit(std::move(frame));
          },
          stop_token, kFrameStallTimeout);
      captured_frames += video_metrics.frames;
      if (!video_metrics.error.empty()) {
        throw std::runtime_error(
            winrt::to_string(winrt::hstring(video_metrics.error)));
      }
      if (video_metrics.stop_requested || stop_token.stop_requested()) {
        break;
      }
      if (!video_metrics.frame_stalled) {
        break;
      }
      std::cerr << "Windows display publisher restarted monitor capture\n";
    }
    const auto pump_metrics = video_pump.finish();
    std::cerr << "[Chatto Desktop capture] Windows display publisher ended: "
              << "stopRequested=" << stop_token.stop_requested()
              << " captured=" << captured_frames
              << " submitted=" << pump_metrics.submitted
              << " published=" << pump_metrics.published
              << " dropped=" << pump_metrics.dropped << "\n";
  } catch (...) {
    stats_reporter.reset();
    room.disconnect();
    throw;
  }
  stats_reporter.reset();
  room.disconnect();
  return 0;
}

} // namespace chatto::capture
