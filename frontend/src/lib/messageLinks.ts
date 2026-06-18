/**
 * Message link URL formats:
 * - `/chat/<serverSegment>/<roomId>/m/<messageId>`
 * - `/chat/<serverSegment>/<roomId>-<roomName>/m/<messageId>`
 * The `m/` prefix distinguishes message URLs from the `[threadId]` route that sits
 * at the same level (thread IDs and message IDs share the same ID space).
 */

import { serverRegistry } from '$lib/state/server/registry.svelte';
import { serverIdToSegment, segmentToServerId } from '$lib/navigation';
import { roomIDFromURLSegment, roomMessagePathForSegment } from '$lib/roomUrls';
import type { ResolvedPathname } from '$app/types';

export interface MessageLink {
  /** URL segment for the server (`-` for origin, hostname for remote). */
  serverSegment: string;
  /** Resolved server ID, or null if the segment doesn't match a registered server. */
  serverId: string | null;
  roomId: string;
  messageId: string;
}

/** Stable message permalink path. Intentionally ID-based even when the room has a name route. */
export function buildMessageLinkPath(
  serverId: string,
  roomId: string,
  messageId: string
): ResolvedPathname {
  return roomMessagePathForSegment(serverIdToSegment(serverId), roomId, messageId);
}

/** Absolute URL for clipboard copy. */
export function buildMessageLinkURL(
  serverId: string,
  roomId: string,
  messageId: string
): string {
  const path = buildMessageLinkPath(serverId, roomId, messageId);

  const server = serverRegistry.getServer(serverId);
  if (server) {
    try {
      return new URL(path, server.url).toString();
    } catch {
      // fall through to window.location.origin
    }
  }

  if (typeof window !== 'undefined') {
    return new URL(path, window.location.origin).toString();
  }

  return path;
}

/**
 * Parse a URL (absolute or relative) and return message link details if it
 * matches the Chatto message link pattern. Returns null for any non-match.
 *
 * Resolves the server segment against the registry when possible so the
 * caller can tell whether the link points at a known (reachable) server.
 */
export function parseMessageLink(input: string): MessageLink | null {
  let pathname: string;
  let hostnameSegment: string | null = null;

  try {
    const url = new URL(input, typeof window !== 'undefined' ? window.location.origin : 'https://_');
    pathname = url.pathname;
    if (typeof window !== 'undefined' && url.host !== window.location.host) {
      hostnameSegment = url.hostname;
    }
  } catch {
    return null;
  }

  const parts = pathname.split('/').filter(Boolean);
  if (parts[0] !== 'chat') return null;

  let serverSegment: string;
  let roomId: string;
  let messageId: string;

  if (parts.length === 5 && parts[3] === 'm') {
    [, serverSegment, roomId, , messageId] = parts;
  } else {
    return null;
  }

  const effectiveSegment = hostnameSegment ?? serverSegment;

  return {
    serverSegment: effectiveSegment,
    serverId: segmentToServerId(effectiveSegment),
    roomId: roomIDFromURLSegment(roomId),
    messageId
  };
}
