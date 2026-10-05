/**
 * Short, human-readable names for the browser and platform that receive push
 * notifications on this device. Settings show them so users can tell which
 * device a server sends notifications to.
 */

export type PushDevice = {
  /** Browser product name, for example `Firefox`, or null when unknown. */
  browser: string | null;
  /** Operating system name, for example `macOS`, or null when unknown. */
  platform: string | null;
};

/** Derives the browser and platform from a user-agent string. */
export function describePushDevice(userAgent: string, maxTouchPoints = 0): PushDevice {
  return { browser: browserName(userAgent), platform: platformName(userAgent, maxTouchPoints) };
}

function browserName(userAgent: string): string | null {
  // Order matters: Chromium-based browsers also report Chrome and Safari, and
  // Chrome also reports Safari.
  if (/\bEdg(e|A|iOS)?\//.test(userAgent)) return 'Edge';
  if (/\b(OPR|Opera)\//.test(userAgent)) return 'Opera';
  if (/\b(Firefox|FxiOS)\//.test(userAgent)) return 'Firefox';
  if (/\b(Chrome|CriOS|Chromium)\//.test(userAgent)) return 'Chrome';
  if (/\bVersion\/[\d.]+.*\bSafari\//.test(userAgent)) return 'Safari';
  return null;
}

function platformName(userAgent: string, maxTouchPoints: number): string | null {
  if (/\biPad\b/.test(userAgent)) return 'iPadOS';
  if (/\b(iPhone|iPod)\b/.test(userAgent)) return 'iOS';
  if (/\bAndroid\b/.test(userAgent)) return 'Android';
  if (/\bCrOS\b/.test(userAgent)) return 'ChromeOS';
  if (/\bWindows\b/.test(userAgent)) return 'Windows';
  // iPadOS Safari reports itself as a Mac but has a touch screen.
  if (/\bMac OS X\b/.test(userAgent)) return maxTouchPoints > 1 ? 'iPadOS' : 'macOS';
  if (/\bLinux\b/.test(userAgent)) return 'Linux';
  return null;
}
