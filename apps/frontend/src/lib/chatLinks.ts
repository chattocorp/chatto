/**
 * Conversation URL grammar, local routes, and share URLs.
 * Only registered-server conversation links qualify for navigation in this SPA.
 */
import { resolve } from '$app/paths';
import type { ResolvedPathname } from '$app/types';
import { serverRegistry } from '$lib/client';
import { serverIdToSegment, segmentToServerId } from '$lib/navigation';

/** The four supported conversation destinations. */
type ChatDestination =
  | { kind: 'room'; roomId: string }
  | { kind: 'thread'; roomId: string; threadRootEventId: string }
  | { kind: 'message'; roomId: string; messageId: string }
  | { kind: 'thread-message'; roomId: string; threadRootEventId: string; messageId: string };

export interface MessageLink {
  /** Local server segment when resolved; otherwise the original segment or URL host. */
  serverSegment: string;
  /** Null for unknown servers; these links cannot load a preview. */
  serverId: string | null;
  roomId: string;
  threadRootEventId?: string;
  messageId: string;
}

export type MessageBodyChatLink = ChatDestination & {
  serverSegment: string;
  serverId: string;
  path: ResolvedPathname;
};

export interface MessageBodyChatLinkOptions {
  origin?: string;
  resolveServerSegment?: (segment: string) => string | null;
  resolveUrlOrigin?: (origin: string) => string | null;
  serverSegmentForId?: (serverId: string) => string;
}

const roomIdPattern = /^(?:R[a-zA-Z0-9]{14}|[a-f0-9]{14})$/;
const eventIdPattern = /^E[a-zA-Z0-9]{14}$/;

/** Parse syntax without consulting the server registry or loading remote data. */
function parseChatLink(
  input: string,
  origin: string
): {
  url: URL;
  serverSegment: string;
  destination: ChatDestination;
} | null {
  let url: URL;
  let parts: string[];
  try {
    url = new URL(input, origin);
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) return null;
    // Decode each segment separately: encoded slashes must never change the route shape.
    parts = url.pathname.replace(/\/$/, '').split('/').slice(1).map(decodeURIComponent);
  } catch {
    return null;
  }
  const [prefix, serverSegment, roomId, fourth, fifth, sixth] = parts;
  if (prefix !== 'chat' || !serverSegment || !roomIdPattern.test(roomId)) return null;
  let destination: ChatDestination;
  if (parts.length === 3) {
    destination = { kind: 'room', roomId };
  } else if (parts.length === 4 && eventIdPattern.test(fourth)) {
    destination = { kind: 'thread', roomId, threadRootEventId: fourth };
  } else if (parts.length === 5 && fourth === 'm' && eventIdPattern.test(fifth)) {
    destination = { kind: 'message', roomId, messageId: fifth };
  } else if (
    parts.length === 6 &&
    eventIdPattern.test(fourth) &&
    fifth === 'm' &&
    eventIdPattern.test(sixth)
  ) {
    destination = { kind: 'thread-message', roomId, threadRootEventId: fourth, messageId: sixth };
  } else {
    return null;
  }
  return { url, serverSegment, destination };
}

/** Unknown URL origins remain external, even when their path names a known server. */
function resolveChatLinkServerId(
  url: URL,
  serverSegment: string,
  origin: string,
  options: MessageBodyChatLinkOptions
): string | null {
  const resolveSegment = options.resolveServerSegment ?? segmentToServerId;
  if (url.origin === origin) return resolveSegment(serverSegment);
  const originServerId = (options.resolveUrlOrigin ?? serverIdForUrlOrigin)(url.origin);
  if (!originServerId) return null;
  return serverSegment === '-' ? originServerId : resolveSegment(serverSegment);
}

/** Absolute links must match a registered origin, including its scheme and port. */
function serverIdForUrlOrigin(origin: string): string | null {
  for (const server of serverRegistry.servers) {
    try {
      if (new URL(server.url).origin === origin) return server.id;
    } catch {
      continue;
    }
  }
  return null;
}

/** Build a SvelteKit route from a home or remote-hostname server segment. */
function destinationPath(serverSegment: string, destination: ChatDestination): ResolvedPathname {
  const params = { serverId: serverSegment, roomId: destination.roomId };
  switch (destination.kind) {
    case 'room':
      return resolve('/chat/[serverId]/[roomId]', params);
    case 'thread':
      return resolve('/chat/[serverId]/[roomId]/[threadId]', {
        ...params,
        threadId: destination.threadRootEventId
      });
    case 'message':
      return resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
        ...params,
        messageId: destination.messageId
      });
    case 'thread-message':
      return resolve('/chat/[serverId]/[roomId]/[threadId]/m/[messageId]', {
        ...params,
        threadId: destination.threadRootEventId,
        messageId: destination.messageId
      });
  }
}

/** Build the local message route, including its thread when supplied. */
export function buildMessageLinkPath(
  serverId: string,
  roomId: string,
  messageId: string,
  threadRootEventId?: string | null
): ResolvedPathname {
  return destinationPath(
    serverIdToSegment(serverId),
    threadRootEventId
      ? { kind: 'thread-message', roomId, messageId, threadRootEventId }
      : { kind: 'message', roomId, messageId }
  );
}

/** Share URLs belong to the owning server, independent of this frontend's origin. */
function buildChatLinkURL(serverId: string, segments: string[]): string {
  const server = serverRegistry.getServer(serverId);
  if (!server) throw new Error('Cannot copy a link for an unknown server');
  return new URL(
    `/chat/-/${segments.map(encodeURIComponent).join('/')}`,
    new URL(server.url).origin
  ).href;
}

/** Absolute room URL on the owning server. Throws if the server is unknown. */
export function buildRoomLinkURL(serverId: string, roomId: string): string {
  return buildChatLinkURL(serverId, [roomId]);
}

/** Absolute message URL on the owning server. Throws if the server is unknown. */
export function buildMessageLinkURL(
  serverId: string,
  roomId: string,
  messageId: string,
  threadRootEventId?: string | null
): string {
  return buildChatLinkURL(
    serverId,
    threadRootEventId ? [roomId, threadRootEventId, 'm', messageId] : [roomId, 'm', messageId]
  );
}

/** Resolve only valid conversation links to registered servers for same-tab navigation. */
export function classifyMessageBodyChatLink(
  input: string,
  options: MessageBodyChatLinkOptions = {}
): MessageBodyChatLink | null {
  const origin =
    options.origin ?? (typeof window !== 'undefined' ? window.location.origin : undefined);
  if (!origin) return null;
  const parsed = parseChatLink(input, origin);
  if (!parsed) return null;
  const serverId = resolveChatLinkServerId(parsed.url, parsed.serverSegment, origin, options);
  if (!serverId) return null;
  const serverSegment = options.serverSegmentForId?.(serverId) ?? serverIdToSegment(serverId);
  return {
    ...parsed.destination,
    serverId,
    serverSegment,
    path: (destinationPath(serverSegment, parsed.destination) +
      parsed.url.search +
      parsed.url.hash) as ResolvedPathname
  };
}

/** Recognize message links for previews, retaining valid links to unknown servers. */
export function parseMessageLink(input: string): MessageLink | null {
  const origin = typeof window !== 'undefined' ? window.location.origin : 'https://_';
  const parsed = parseChatLink(input, origin);
  if (!parsed || !('messageId' in parsed.destination)) return null;
  const serverId = resolveChatLinkServerId(parsed.url, parsed.serverSegment, origin, {});
  return {
    serverId,
    serverSegment: serverId
      ? serverIdToSegment(serverId)
      : parsed.url.origin === origin
        ? parsed.serverSegment
        : parsed.url.host,
    roomId: parsed.destination.roomId,
    messageId: parsed.destination.messageId,
    threadRootEventId:
      'threadRootEventId' in parsed.destination ? parsed.destination.threadRootEventId : undefined
  };
}
