/**
 * Diagnostic output for development. Browsers hide `console.debug` by
 * default, but Node writes it to stdout, where per-event diagnostics would
 * flood a bot's logs. Debug output is therefore on in browsers and off in
 * other hosts until {@link setDebugLogging} enables it.
 */

let enabled = typeof window !== 'undefined';

/** Turn client debug output on or off. */
export function setDebugLogging(value: boolean): void {
  enabled = value;
}

/** Write client debug output when it is on. */
export function debugLog(...args: unknown[]): void {
  if (enabled) console.debug(...args);
}
