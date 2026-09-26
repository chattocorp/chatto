import type { CustomUserStatus } from '$lib/state/userProfiles.svelte';

const MAX_TIMEOUT_DELAY_MS = 2_147_483_647;

/** Schedule expiry without browser timeout overflow; cancellation also fences callbacks. */
export function scheduleCustomStatusExpiry(
  status: CustomUserStatus | null | undefined,
  onExpire: () => void
): () => void {
  const expiresAt = status?.expiresAt;
  if (!expiresAt) return () => {};
  let timeout: ReturnType<typeof setTimeout> | undefined;
  let cancelled = false;
  const schedule = (fromTimer = false) => {
    if (cancelled) return;
    const expiresAtMs = Date.parse(expiresAt);
    if (Number.isNaN(expiresAtMs)) return;
    const delay = expiresAtMs - Date.now();
    if (delay <= 0) {
      if (fromTimer) onExpire();
      else
        timeout = setTimeout(() => {
          if (!cancelled) onExpire();
        }, 0);
      return;
    }
    timeout = setTimeout(() => schedule(true), Math.min(delay, MAX_TIMEOUT_DELAY_MS));
  };
  schedule();
  return () => {
    cancelled = true;
    if (timeout) clearTimeout(timeout);
  };
}
