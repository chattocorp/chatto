/**
 * Cross-tab session synchronization via BroadcastChannel.
 *
 * When a user logs out in one tab, this notifies all other same-origin tabs
 * instantly (no server round-trip needed). This complements the server-side
 * SessionTerminatedEvent which handles cross-device and admin-initiated logout.
 */

const CHANNEL_NAME = 'chatto-session';

let channel: BroadcastChannel | null = null;

/**
 * Initialize the session channel listener.
 * When another tab broadcasts a logout, calls the provided callback.
 * Returns a cleanup function to close the channel.
 */
export function initSessionChannel(onLogout: () => void): () => void {
  channel = new BroadcastChannel(CHANNEL_NAME);
  channel.onmessage = (event) => {
    if (event.data?.type === 'logout') {
      onLogout();
    }
  };

  return () => {
    channel?.close();
    channel = null;
  };
}

/**
 * Notify other tabs that the user has logged out.
 * Creates a temporary channel for sending because the calling tab
 * is about to navigate away (its persistent channel may already be torn down).
 */
export function notifyLogout(): void {
  const ch = new BroadcastChannel(CHANNEL_NAME);
  ch.postMessage({ type: 'logout' });
  ch.close();
}
