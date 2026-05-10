const STORAGE_KEY = 'chatto:secondarySidebarWidth';

export const SECONDARY_SIDEBAR_DEFAULT_WIDTH = 256;
export const SECONDARY_SIDEBAR_MIN_WIDTH = 200;
export const SECONDARY_SIDEBAR_MAX_WIDTH = 480;

export function getSecondarySidebarWidth(): number {
  if (typeof localStorage === 'undefined') {
    return SECONDARY_SIDEBAR_DEFAULT_WIDTH;
  }
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const value = parseFloat(stored);
      if (
        !isNaN(value) &&
        value >= SECONDARY_SIDEBAR_MIN_WIDTH &&
        value <= SECONDARY_SIDEBAR_MAX_WIDTH
      ) {
        return value;
      }
    }
  } catch {
    // Ignore storage errors
  }
  return SECONDARY_SIDEBAR_DEFAULT_WIDTH;
}

export function setSecondarySidebarWidth(width: number): void {
  const clamped = Math.min(
    SECONDARY_SIDEBAR_MAX_WIDTH,
    Math.max(SECONDARY_SIDEBAR_MIN_WIDTH, width)
  );
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, String(clamped));
  } catch {
    // Ignore storage errors
  }
}
