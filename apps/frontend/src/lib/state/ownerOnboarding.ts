/**
 * One-shot hand-off from first-run setup to the origin server's chat shell.
 *
 * Setup marks the onboarding as pending after it creates the owner account.
 * The chat shell takes the mark once, when the new owner's origin session is
 * ready, and then shows the owner welcome. The mark is in memory only: a page
 * reload drops it, and the owner can still use the normal controls.
 */
let pending: { userId?: string } | null = null;

/** Request the welcome, optionally bound to the account created by sign-in. */
export function markOwnerOnboardingPending(userId?: string): void {
  pending = { userId };
}

/** Consume the request; an account-bound mark only matches that account. */
export function takeOwnerOnboarding(userId?: string): boolean {
  const mark = pending;
  pending = null;
  return mark !== null && (mark.userId === undefined || mark.userId === userId);
}
