/**
 * Leaves the app for a provider flow in the current window. A separate module
 * lets tests observe the navigation without leaving the test page.
 */
export function navigateToProvider(url: string): void {
  window.location.href = url;
}
