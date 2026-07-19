/** Updates the installed app badge from Chatto's authoritative notification count. */
export async function updateAppBadge(notificationCount: number): Promise<void> {
  if (typeof navigator === 'undefined' || !navigator.setAppBadge) return;

  try {
    await navigator.setAppBadge(notificationCount);
  } catch {
    // Badge support and permission vary by browser and installation context.
  }
}
