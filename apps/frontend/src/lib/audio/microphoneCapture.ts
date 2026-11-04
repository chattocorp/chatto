import type { AudioCaptureOptions } from 'livekit-client';

/** Keep calls and local microphone recordings on the same browser capture settings. */
export function microphoneCaptureOptions(deviceId = ''): AudioCaptureOptions {
  return {
    // Prefer the saved input without failing when that device is unavailable.
    deviceId: { ideal: deviceId || 'default' },
    channelCount: { ideal: 1 },
    autoGainControl: true,
    echoCancellation: true,
    noiseSuppression: true,
    // Match LiveKit's capture default. Browsers ignore unsupported constraints.
    voiceIsolation: true
  };
}
