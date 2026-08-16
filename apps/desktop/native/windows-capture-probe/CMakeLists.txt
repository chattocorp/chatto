# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: Apache-2.0

cmake_minimum_required(VERSION 3.25)

project(chatto-windows-capture-probe LANGUAGES CXX)

set(CHATTO_LIVEKIT_VERSION "1.7.0-chatto.3")
set(CHATTO_LIVEKIT_SHA256 "a7a26545c667d4ce8da1f1f13924c32c7613db375db320e97c99f828cfdaff01")
set(CHATTO_LIVEKIT_SDK_ROOT "" CACHE PATH "Path to an extracted LiveKit C++ SDK")
if(NOT CHATTO_LIVEKIT_SDK_ROOT)
  set(_livekit_archive "${CMAKE_BINARY_DIR}/_deps/livekit-sdk-windows-x64-${CHATTO_LIVEKIT_VERSION}.zip")
  set(_livekit_extract "${CMAKE_BINARY_DIR}/_deps/livekit-sdk-${CHATTO_LIVEKIT_VERSION}")
  set(CHATTO_LIVEKIT_SDK_ROOT
    "${_livekit_extract}/livekit-sdk-windows-x64-${CHATTO_LIVEKIT_VERSION}")
  if(NOT EXISTS "${CHATTO_LIVEKIT_SDK_ROOT}/lib/livekit.lib")
    file(MAKE_DIRECTORY "${CMAKE_BINARY_DIR}/_deps")
    file(DOWNLOAD
      "https://github.com/chattocorp/client-sdk-cpp/releases/download/v${CHATTO_LIVEKIT_VERSION}/livekit-sdk-windows-x64-${CHATTO_LIVEKIT_VERSION}.zip"
      "${_livekit_archive}"
      EXPECTED_HASH "SHA256=${CHATTO_LIVEKIT_SHA256}"
      TLS_VERIFY ON
      SHOW_PROGRESS
    )
    file(MAKE_DIRECTORY "${_livekit_extract}")
    file(ARCHIVE_EXTRACT INPUT "${_livekit_archive}" DESTINATION "${_livekit_extract}")
  endif()
endif()
# find_package caches LiveKit_DIR independently of our versioned archive path.
# Clear it so changing the pin also updates an existing helper build directory.
unset(LiveKit_DIR CACHE)
find_package(LiveKit CONFIG REQUIRED PATHS "${CHATTO_LIVEKIT_SDK_ROOT}" NO_DEFAULT_PATH)

set(CHATTO_NV_CODEC_HEADERS_VERSION "13.0.19.1")
set(CHATTO_NV_CODEC_HEADERS_SHA256 "eab9d02d461035a4baded23d4f2e2834e5b17934820b79cea45baaf0c85383eb")
set(_nv_codec_archive "${CMAKE_BINARY_DIR}/_deps/nv-codec-headers-${CHATTO_NV_CODEC_HEADERS_VERSION}.tar.gz")
set(_nv_codec_extract "${CMAKE_BINARY_DIR}/_deps/nv-codec-headers-${CHATTO_NV_CODEC_HEADERS_VERSION}")
set(CHATTO_NV_CODEC_HEADERS_ROOT
  "${_nv_codec_extract}/nv-codec-headers-${CHATTO_NV_CODEC_HEADERS_VERSION}")
if(NOT EXISTS "${CHATTO_NV_CODEC_HEADERS_ROOT}/include/ffnvcodec/nvEncodeAPI.h")
  file(MAKE_DIRECTORY "${CMAKE_BINARY_DIR}/_deps")
  file(DOWNLOAD
    "https://github.com/FFmpeg/nv-codec-headers/releases/download/n${CHATTO_NV_CODEC_HEADERS_VERSION}/nv-codec-headers-${CHATTO_NV_CODEC_HEADERS_VERSION}.tar.gz"
    "${_nv_codec_archive}"
    EXPECTED_HASH "SHA256=${CHATTO_NV_CODEC_HEADERS_SHA256}"
    TLS_VERIFY ON
    SHOW_PROGRESS
  )
  file(MAKE_DIRECTORY "${_nv_codec_extract}")
  file(ARCHIVE_EXTRACT INPUT "${_nv_codec_archive}" DESTINATION "${_nv_codec_extract}")
endif()

add_executable(chatto-windows-capture-probe
  src/audio_capture.cpp
  src/h264_encoder.cpp
  src/nvenc_encoder.cpp
  src/livekit_publisher.cpp
  src/main.cpp
  src/preview_window.cpp
  src/video_capture.cpp
  src/window_sources.cpp
)

target_compile_features(chatto-windows-capture-probe PRIVATE cxx_std_20)
target_compile_definitions(chatto-windows-capture-probe PRIVATE
  NOMINMAX
  UNICODE
  WIN32_LEAN_AND_MEAN
  _UNICODE
)
target_compile_options(chatto-windows-capture-probe PRIVATE
  /EHsc
  /permissive-
  /utf-8
  /W4
  /WX
)
target_include_directories(chatto-windows-capture-probe PRIVATE
  "${CHATTO_NV_CODEC_HEADERS_ROOT}/include"
)
target_link_libraries(chatto-windows-capture-probe PRIVATE
  d3d11
  bcrypt
  dwmapi
  dxgi
  mf
  mfplat
  mfuuid
  mmdevapi
  ole32
  psapi
  user32
  windowsapp
  LiveKit::livekit
)

add_custom_command(TARGET chatto-windows-capture-probe POST_BUILD
  COMMAND ${CMAKE_COMMAND} -E copy_if_different
    "${CHATTO_LIVEKIT_SDK_ROOT}/bin/livekit.dll"
    "$<TARGET_FILE_DIR:chatto-windows-capture-probe>"
  COMMAND ${CMAKE_COMMAND} -E copy_if_different
    "${CHATTO_LIVEKIT_SDK_ROOT}/bin/livekit_ffi.dll"
    "$<TARGET_FILE_DIR:chatto-windows-capture-probe>"
)

include(CTest)

if(BUILD_TESTING)
  add_executable(chatto-windows-latest-frame-queue-tests
    tests/latest_frame_queue_test.cpp
  )
  target_compile_features(chatto-windows-latest-frame-queue-tests PRIVATE cxx_std_20)
  target_compile_options(chatto-windows-latest-frame-queue-tests PRIVATE
    /EHsc
    /permissive-
    /utf-8
    /W4
    /WX
  )
  add_test(
    NAME latest-frame-queue-tests
    COMMAND chatto-windows-latest-frame-queue-tests
  )

  add_executable(chatto-windows-capture-probe-tests
    tests/window_sources_test.cpp
    src/window_sources.cpp
  )
  target_compile_features(chatto-windows-capture-probe-tests PRIVATE cxx_std_20)
  target_compile_definitions(chatto-windows-capture-probe-tests PRIVATE
    NOMINMAX
    UNICODE
    WIN32_LEAN_AND_MEAN
    _UNICODE
  )
  target_compile_options(chatto-windows-capture-probe-tests PRIVATE
    /EHsc
    /permissive-
    /utf-8
    /W4
    /WX
  )
  target_link_libraries(chatto-windows-capture-probe-tests PRIVATE bcrypt dwmapi)
  add_test(NAME window-source-tests COMMAND chatto-windows-capture-probe-tests)

  add_executable(chatto-windows-h264-encoder-tests
    tests/h264_encoder_test.cpp
    src/h264_encoder.cpp
    src/nvenc_encoder.cpp
  )
  target_compile_features(chatto-windows-h264-encoder-tests PRIVATE cxx_std_20)
  target_compile_definitions(chatto-windows-h264-encoder-tests PRIVATE
    NOMINMAX
    UNICODE
    WIN32_LEAN_AND_MEAN
    _UNICODE
  )
  target_compile_options(chatto-windows-h264-encoder-tests PRIVATE
    /EHsc
    /permissive-
    /utf-8
    /W4
    /WX
  )
  target_include_directories(chatto-windows-h264-encoder-tests PRIVATE src)
  target_include_directories(chatto-windows-h264-encoder-tests PRIVATE
    "${CHATTO_NV_CODEC_HEADERS_ROOT}/include"
  )
  target_link_libraries(chatto-windows-h264-encoder-tests PRIVATE
    d3d11
    dxgi
    mf
    mfplat
    mfuuid
  )
  add_test(NAME h264-encoder-tests COMMAND chatto-windows-h264-encoder-tests)
endif()
