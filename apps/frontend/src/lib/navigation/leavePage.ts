/**
 * Load a URL in this window with a full page load, for example to send a
 * sign-in window to a provider. Specs mock this module to observe the target
 * without leaving the test page.
 */
export function leavePage(url: string): void {
  window.location.href = url;
}
