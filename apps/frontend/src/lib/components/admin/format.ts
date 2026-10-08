export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

export function formatNumber(n: number): string {
  return n.toLocaleString();
}

const goDurationUnitMs: Record<string, number> = {
  ns: 1e-6,
  us: 1e-3,
  µs: 1e-3,
  μs: 1e-3,
  ms: 1,
  s: 1000
};

/**
 * Formats a single-unit Go duration string, such as `543.432µs`, as a
 * rounded value with the most readable unit. Returns other input, such as
 * `1m2.5s`, unchanged.
 */
export function formatGoDuration(duration: string): string {
  const match = /^(\d+(?:\.\d+)?)(ns|us|µs|μs|ms|s)$/.exec(duration);
  if (!match) return duration;
  const ms = Number(match[1]) * goDurationUnitMs[match[2]];
  // Choose the unit after rounding, so 999.9 µs becomes 1 ms, not 1000 µs.
  const us = Math.round(ms * 1000);
  if (us < 1000) return `${us} µs`;
  const roundedMs = parseFloat(ms.toFixed(ms < 10 ? 1 : 0));
  if (roundedMs < 1000) return `${roundedMs} ms`;
  return `${parseFloat((ms / 1000).toFixed(2))} s`;
}
