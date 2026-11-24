/**
 * One-shot hand-off from first-run setup to the origin server's chat shell.
 *
 * Setup marks the onboarding as pending after it creates the owner account.
 * The chat shell takes the mark once, when the new owner's origin session is
 * ready, and then shows the owner welcome. The mark is in memory only: a page
 * reload drops it, and the owner can still use the normal controls.
 */
let pending = false;

/** Request the owner welcome on the next origin server chat shell. */
export function markOwnerOnboardingPending(): void {
  pending = true;
}

/** Return whether the owner welcome was pending, and clear the request. */
export function takeOwnerOnboarding(): boolean {
  const wasPending = pending;
  pending = false;
  return wasPending;
}
