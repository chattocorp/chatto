/**
 * Best-effort detection of this device's IANA time zone.
 * Returns `null` when the platform cannot resolve a zone.
 */
export function deviceTimezone(): string | null {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || null;
  } catch {
    return null;
  }
}

/**
 * Tracks best-effort time-zone reports for one mounted chat root. This state
 * is intentionally not reactive: a failed request must not restart the
 * reporting effect by itself.
 */
export function createDeviceTimezoneReportTracker() {
  const reported = new Set<string>();

  return {
    begin(key: string): boolean {
      if (reported.has(key)) return false;
      reported.add(key);
      return true;
    },
    allowRetry(key: string): void {
      reported.delete(key);
    }
  };
}
