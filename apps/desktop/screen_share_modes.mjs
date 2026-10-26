/** Native publisher modes. A share keeps its selected object until it stops. */
export const screenShareModes = Object.freeze(
  [
    ['default', 'Default: Tuned H.264 (Automatic VBR)', 'h264', false, true, 'auto'],
    ['hevc', 'Prefer HEVC', 'h265', true, false],
    ['h264', 'Use H.264', 'h264', true, false],
    ['single', 'Test H.264 Single Layer (8 Mbps, Resolution First)', 'h264', false, false],
    ['average', 'Test Tuned H.264 (8 Mbps, Resolution First)', 'h264', false, true, 'average'],
    ['vbr', 'Test Tuned H.264 VBR (6 Mbps Target, 8 Mbps Max)', 'h264', false, true, 'vbr']
  ].map(([id, label, codec, simulcast, tunedH264, rateControl]) =>
    Object.freeze({ id, label, codec, simulcast, tunedH264, rateControl })
  )
);

/** VBR when available, with native average-rate fallback on unsupported Macs. */
export const defaultScreenShareMode = screenShareModes[0];
