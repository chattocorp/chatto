import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';
import { MessageSearchOrder, type MessageSearchStore } from '$lib/state/server/messageSearch';

/**
 * The message-search query that selects messages by one login, such as
 * `from:alice`. A login with other characters is quoted.
 */
export function messagesFromQuery(login: string): string {
  return /^[\p{L}\p{N}_.-]+$/u.test(login)
    ? `from:${login}`
    : `from:"${login.replaceAll('"', '')}"`;
}

/**
 * Start a newest-first server search for a user's messages and open the search
 * page. The query stays visible and editable there.
 */
export function searchUserMessages(serverId: string, store: MessageSearchStore, login: string) {
  void store.search({ query: messagesFromQuery(login), order: MessageSearchOrder.NEWEST });
  void goto(resolve('/chat/[serverId]/search', { serverId: serverIdToSegment(serverId) }));
}
