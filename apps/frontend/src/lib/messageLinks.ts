/**
 * Message link URL format: `/chat/<serverSegment>/<roomId>/m/<messageId>`.
 * The `m/` prefix distinguishes message URLs from the `[threadId]` route that sits
 * at the same level (thread IDs and message IDs share the same ID space).
 */

import { resolve } from '$app/paths';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { serverIdToSegment, segmentToServerId } from '$lib/navigation';
import { toast } from '$lib/ui/toast';

export interface MessageLink {
  /** URL segment for the server (`-` for origin, hostname for remote). */
  serverSegment: string;
  /** Resolved server ID, or null if the segment doesn't match a registered server. */
  serverId: string | null;
  roomId: string;
  messageId: string;
}

export type MessageBodyChatLink =
  | {
      kind: 'room';
      serverSegment: string;
      serverId: string;
      roomId: string;
      path: string;
    }
  | {
      kind: 'thread';
      serverSegment: string;
      serverId: string;
      roomId: string;
      threadRootEventId: string;
      path: string;
    }
  | {
      kind: 'message';
      serverSegment: string;
      serverId: string;
      roomId: string;
      messageId: string;
      path: string;
    };

export interface MessageBodyChatLinkOptions {
  origin?: string;
  resolveServerSegment?: (segment: string) => string | null;
  resolveUrlOrigin?: (origin: string) => string | null;
  serverSegmentForId?: (serverId: string) => string;
}

const channelRoomIdPattern = /^R[a-zA-Z0-9]{14}$/;
const dmRoomIdPattern = /^[a-f0-9]{14}$/;
const eventIdPattern = /^E[a-zA-Z0-9]{14}$/;

function isMessageRoomId(value: string): boolean {
  return channelRoomIdPattern.test(value) || dmRoomIdPattern.test(value);
}

function serverIdForUrlOrigin(origin: string): string | null {
  for (const server of serverRegistry.servers) {
    try {
      if (new URL(server.url).origin === origin) {
        return server.id;
      }
    } catch {
      continue;
    }
  }

  return null;
}

function routeSuffix(url: URL): string {
  return `${url.search}${url.hash}`;
}

function roomPath(
  serverId: string,
  roomId: string,
  suffix: string,
  options: MessageBodyChatLinkOptions
): string {
  const serverSegment = options.serverSegmentForId?.(serverId) ?? serverIdToSegment(serverId);
  return `${resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId })}${suffix}`;
}

function threadPath(
  serverId: string,
  roomId: string,
  threadRootEventId: string,
  suffix: string,
  options: MessageBodyChatLinkOptions
): string {
  const serverSegment = options.serverSegmentForId?.(serverId) ?? serverIdToSegment(serverId);
  return `${resolve('/chat/[serverId]/[roomId]/[threadId]', {
    serverId: serverSegment,
    roomId,
    threadId: threadRootEventId
  })}${suffix}`;
}

function messagePath(
  serverId: string,
  roomId: string,
  messageId: string,
  suffix: string,
  options: MessageBodyChatLinkOptions
): string {
  const serverSegment = options.serverSegmentForId?.(serverId) ?? serverIdToSegment(serverId);
  return `${resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
    serverId: serverSegment,
    roomId,
    messageId
  })}${suffix}`;
}

function resolveChatLinkServerId(
  url: URL,
  serverSegment: string,
  origin: string,
  options: MessageBodyChatLinkOptions
): string | null {
  const resolveServerSegment = options.resolveServerSegment ?? segmentToServerId;

  if (url.origin === origin) {
    return resolveServerSegment(serverSegment);
  }

  const resolveUrlOrigin = options.resolveUrlOrigin ?? serverIdForUrlOrigin;
  const urlOriginServerId = resolveUrlOrigin(url.origin);
  if (!urlOriginServerId) return null;

  if (serverSegment === '-') {
    return urlOriginServerId;
  }

  return resolveServerSegment(serverSegment);
}

export function buildMessageLinkPath(serverId: string, roomId: string, messageId: string): string {
  return resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
    serverId: serverIdToSegment(serverId),
    roomId,
    messageId
  });
}

/** Absolute URL for clipboard copy. */
export function buildMessageLinkURL(serverId: string, roomId: string, messageId: string): string {
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

export async function copyMessageLinkToClipboard(
  serverId: string,
  roomId: string,
  messageId: string
): Promise<void> {
  try {
    await navigator.clipboard.writeText(buildMessageLinkURL(serverId, roomId, messageId));
    toast.success('Message link copied');
  } catch {
    toast.error('Failed to copy link');
  }
}

/**
 * Classify message-body URLs that may navigate in the current tab.
 *
 * This deliberately allows only registered-server Chatto room, thread, and
 * message URLs with Chatto-looking IDs. Other URLs, such as settings or admin
 * pages, should keep opening out-of-band like external links.
 */
export function classifyMessageBodyChatLink(
  input: string,
  options: MessageBodyChatLinkOptions = {}
): MessageBodyChatLink | null {
  const origin =
    options.origin ?? (typeof window !== 'undefined' ? window.location.origin : undefined);
  if (!origin) return null;

  let url: URL;
  try {
    url = new URL(input, origin);
  } catch {
    return null;
  }

  const parts = url.pathname.split('/').filter(Boolean);
  if (parts[0] !== 'chat') return null;
  if (parts.length !== 3 && parts.length !== 4 && parts.length !== 5) return null;

  const [, serverSegment, roomId] = parts;
  if (!serverSegment || !isMessageRoomId(roomId)) return null;

  const serverId = resolveChatLinkServerId(url, serverSegment, origin, options);
  if (!serverId) return null;

  const suffix = routeSuffix(url);
  const localServerSegment = options.serverSegmentForId?.(serverId) ?? serverIdToSegment(serverId);

  if (parts.length === 3) {
    return {
      kind: 'room',
      serverSegment: localServerSegment,
      serverId,
      roomId,
      path: roomPath(serverId, roomId, suffix, options)
    };
  }

  if (parts.length === 4) {
    const threadRootEventId = parts[3];
    if (!eventIdPattern.test(threadRootEventId)) return null;
    return {
      kind: 'thread',
      serverSegment: localServerSegment,
      serverId,
      roomId,
      threadRootEventId,
      path: threadPath(serverId, roomId, threadRootEventId, suffix, options)
    };
  }

  const [, , , marker, messageId] = parts;
  if (marker !== 'm' || !eventIdPattern.test(messageId)) return null;
  return {
    kind: 'message',
    serverSegment: localServerSegment,
    serverId,
    roomId,
    messageId,
    path: messagePath(serverId, roomId, messageId, suffix, options)
  };
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
    const url = new URL(
      input,
      typeof window !== 'undefined' ? window.location.origin : 'https://_'
    );
    pathname = url.pathname;
    if (typeof window !== 'undefined' && url.host !== window.location.host) {
      hostnameSegment = url.hostname;
    }
  } catch {
    return null;
  }

  const parts = pathname.split('/').filter(Boolean);
  // Expected: ['chat', serverSegment, roomId, 'm', messageId]
  if (parts.length !== 5) return null;
  if (parts[0] !== 'chat' || parts[3] !== 'm') return null;

  const [, serverSegment, roomId, , messageId] = parts;
  const effectiveSegment = hostnameSegment ?? serverSegment;

  return {
    serverSegment: effectiveSegment,
    serverId: segmentToServerId(effectiveSegment),
    roomId,
    messageId
  };
}
