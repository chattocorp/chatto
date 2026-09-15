/** Browser-local microphone effects. Gains are dB; compressor amount is 0–100. */
export interface MicrophoneEffects {
  lowCut: boolean;
  equalizer: boolean;
  bass: number;
  mid: number;
  treble: number;
  compressor: boolean;
  amount: number;
}

/** Bound inputs before they reach AudioParams; omitted values produce fresh defaults. */
export function normalizeMicrophoneEffects(value?: unknown): MicrophoneEffects {
  const raw = value && typeof value === 'object' ? (value as Partial<MicrophoneEffects>) : {};
  const bounded = (value: unknown, min: number, max: number, fallback: number) =>
    typeof value === 'number' && Number.isFinite(value)
      ? Math.round(Math.max(min, Math.min(max, value)))
      : fallback;
  return {
    lowCut: raw.lowCut === true,
    equalizer: raw.equalizer === true,
    bass: bounded(raw.bass, -6, 6, 0),
    mid: bounded(raw.mid, -6, 6, 0),
    treble: bounded(raw.treble, -6, 6, 0),
    compressor: raw.compressor === true,
    amount: bounded(raw.amount, 0, 100, 50)
  };
}

/** User-facing processing choices; the noise gate is configured separately. */
export type MicrophoneProcessingPreset = 'none' | 'subtle' | 'strong';

/** Return fresh effect settings so callers cannot mutate shared preset values. */
export function microphoneEffectsForPreset(preset: MicrophoneProcessingPreset): MicrophoneEffects {
  const enabled = preset !== 'none';
  const strong = preset === 'strong';
  return {
    lowCut: enabled,
    equalizer: enabled,
    bass: enabled ? (strong ? -3 : -1) : 0,
    mid: enabled ? (strong ? 2 : 1) : 0,
    treble: enabled ? (strong ? 3 : 1) : 0,
    compressor: enabled,
    amount: enabled ? (strong ? 55 : 15) : 50
  };
}
