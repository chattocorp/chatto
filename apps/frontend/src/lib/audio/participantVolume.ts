/**
 * Map listener volume onto a perceptual gain curve: 50% is -10 dB,
 * 100% is unity, and 200% is +10 dB. This approximates relative loudness;
 * it does not measure or normalize the source. Zero remains exact silence.
 * Browsers without Web Audio cannot amplify, but retain the same lower curve.
 */
export function participantVolumeGain(percent: number, boostAvailable = true): number {
  const volume = Number.isFinite(percent) ? percent : 100;
  const ratio = Math.max(0, Math.min(boostAvailable ? 200 : 100, volume)) / 100;
  return ratio ** (Math.log2(10) / 2);
}
