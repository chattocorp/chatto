/** Detect iOS, including iPads with a desktop user agent. */
export function isIosBrowserContext(): boolean {
  if (typeof navigator === 'undefined') return false;

  const platform = navigator.platform;
  const userAgent = navigator.userAgent;
  const touchCapableMac = platform === 'MacIntel' && navigator.maxTouchPoints > 1;
  return /iPad|iPhone|iPod/.test(userAgent) || touchCapableMac;
}

/** Detect an installed Home Screen app. */
export function isStandaloneDisplayMode(): boolean {
  if (typeof window === 'undefined') return false;

  return (
    window.matchMedia?.('(display-mode: standalone)').matches === true ||
    (navigator as Navigator & { standalone?: boolean }).standalone === true
  );
}
