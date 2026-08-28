export const SLOW_MODE_PRESETS = [
  0, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 21600
] as const;

export function formatSlowModeInterval(seconds: number, locale: string): string {
  const normalized = Math.max(0, Math.floor(seconds));
  const [value, unit] =
    normalized > 0 && normalized % 3600 === 0
      ? [normalized / 3600, 'hour' as const]
      : normalized > 0 && normalized % 60 === 0
        ? [normalized / 60, 'minute' as const]
        : [normalized, 'second' as const];
  return new Intl.NumberFormat(locale, {
    style: 'unit',
    unit,
    unitDisplay: 'long'
  }).format(value);
}

export function formatSlowModeCountdown(totalSeconds: number): string {
  const seconds = Math.max(0, Math.ceil(totalSeconds));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;
  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}`;
  }
  return `${minutes}:${String(remainder).padStart(2, '0')}`;
}

export function slowModeRemainingSeconds(deadlineMs: number | null, nowMs = Date.now()): number {
  return deadlineMs === null ? 0 : Math.max(0, Math.ceil((deadlineMs - nowMs) / 1000));
}
