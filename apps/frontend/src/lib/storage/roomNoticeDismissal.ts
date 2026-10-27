/** Device-local dismissal identity. Notice content is never persisted. */
import { serverStorageKey } from '@chatto/client/storage/serverStorage';

export function roomNoticeDismissalKey(serverId: string, viewerId: string, roomId: string): string {
  return serverStorageKey(serverId, `viewer:${viewerId}:room:${roomId}:noticeDismissed`);
}

/** Storage can be unavailable in private browsing or server rendering. */
export function readNoticeDismissal(key: string): string {
  try {
    return localStorage.getItem(key) ?? '';
  } catch {
    return '';
  }
}

export function writeNoticeDismissal(key: string, publicationId: string): void {
  try {
    if (publicationId) localStorage.setItem(key, publicationId);
    else localStorage.removeItem(key);
  } catch {
    /* Dismissal still works for the mounted view without storage. */
  }
}
