import { resolve } from '$app/paths';
import { generateState } from './pkce';

export interface AuthorizationWindow {
  readonly messageSource: Window | null;
  close(): Promise<void>;
  isClosed(): Promise<boolean>;
  navigate(url: string): Promise<void>;
  detachOpener(): void;
}

/**
 * Same-origin page that an authorization window opens on. The page reads its
 * target from a launch record and replaces itself with that URL.
 */
export const AUTHORIZATION_LAUNCH_PATH = '/servers/authorize';

/** A launch page ignores records older than this. */
export const AUTHORIZATION_LAUNCH_TTL_MS = 5 * 60 * 1000;

const LAUNCH_STORAGE_PREFIX = 'chatto:oauth-launch:';

/** Target URL for one authorization window, stored until its launch page reads it. */
export type AuthorizationLaunchRecord = { url: string; createdAt: number };

/** Return the device storage key of the launch record for one window. */
export function authorizationLaunchStorageKey(launchId: string): string {
  return LAUNCH_STORAGE_PREFIX + launchId;
}

/**
 * Open an authorization window while a user gesture is active. Returns `null`
 * when the browser blocks the window.
 *
 * The window opens on the same-origin launch page, not on `about:blank`.
 * Firefox for Android opens a window from an installed web app in a separate
 * Custom Tab with the URL known at open time, and the returned `Window` does
 * not control that tab. `navigate` therefore gives the target to the launch
 * page through `localStorage`, which the separate tab shares. If storage is
 * unavailable, `navigate` sets the window's location directly.
 *
 * The record stays until `close`, because more than one launch page can read
 * it; see `readAuthorizationLaunchTarget`. Opening a window also removes
 * expired records that a closed client left behind.
 */
export function openAuthorizationWindow(
  target: string,
  features: string
): AuthorizationWindow | null {
  const launchId = generateState();
  const launchUrl = `${window.location.origin}${resolve(AUTHORIZATION_LAUNCH_PATH)}#${launchId}`;
  const popup = window.open(launchUrl, target, features);
  if (!popup) return null;
  removeExpiredLaunchRecords(Date.now());

  const key = authorizationLaunchStorageKey(launchId);
  return {
    messageSource: popup,
    close: async () => {
      try {
        localStorage.removeItem(key);
      } catch {
        // Unavailable storage cannot hold a record to remove.
      }
      if (!popup.closed) popup.close();
    },
    isClosed: async () => popup.closed,
    navigate: async (url) => {
      const record: AuthorizationLaunchRecord = { url, createdAt: Date.now() };
      try {
        localStorage.setItem(key, JSON.stringify(record));
      } catch {
        popup.location.replace(url);
      }
    },
    detachOpener: () => {
      popup.opener = null;
    }
  };
}

function removeExpiredLaunchRecords(now: number): void {
  try {
    for (let index = localStorage.length - 1; index >= 0; index--) {
      const key = localStorage.key(index);
      if (!key?.startsWith(LAUNCH_STORAGE_PREFIX)) continue;
      try {
        parseLaunchTarget(localStorage.getItem(key), now);
      } catch {
        localStorage.removeItem(key);
      }
    }
  } catch {
    // Unavailable storage cannot hold records to remove.
  }
}

/**
 * Read the launch record for `launchId` without removing it. Returns the
 * target URL only for a fresh HTTP or HTTPS record, and `null` while no record
 * exists. Throws for an invalid or expired record.
 *
 * Firefox for Android can load the launch page twice: in the visible Custom
 * Tab and in the detached window that `window.open` returned. A read that
 * removed the record could let the detached window take the target from the
 * visible tab.
 */
export function readAuthorizationLaunchTarget(launchId: string, now = Date.now()): string | null {
  return parseLaunchTarget(localStorage.getItem(authorizationLaunchStorageKey(launchId)), now);
}

function parseLaunchTarget(raw: string | null, now: number): string | null {
  if (raw === null) return null;
  const record = JSON.parse(raw) as Partial<AuthorizationLaunchRecord>;
  if (typeof record.url !== 'string' || typeof record.createdAt !== 'number') {
    throw new Error('Invalid authorization launch record.');
  }
  if (now - record.createdAt > AUTHORIZATION_LAUNCH_TTL_MS) {
    throw new Error('Expired authorization launch record.');
  }
  const url = new URL(record.url);
  if (url.protocol !== 'https:' && url.protocol !== 'http:') {
    throw new Error('Unsupported authorization launch URL.');
  }
  return url.href;
}

/**
 * Replace the launch page with its target. Replacement keeps the launch page
 * out of history, so Back from the target does not return to it.
 */
export function replaceLaunchPage(target: string): void {
  window.location.replace(target);
}

/** Size an authorization popup for the form while keeping it within the screen. */
export function authorizationWindowFeatures(owner: Window): string {
  const width = Math.max(1, Math.min(560, owner.screen.availWidth - 32));
  const height = Math.max(1, Math.min(760, owner.screen.availHeight - 100));
  const left = Math.max(0, Math.round(owner.screenX + (owner.outerWidth - width) / 2));
  const top = Math.max(0, Math.round(owner.screenY + (owner.outerHeight - height) / 2));
  return `popup,width=${width},height=${height},left=${left},top=${top}`;
}
