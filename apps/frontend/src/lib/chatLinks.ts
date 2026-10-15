/**
 * Chatto route rewriting, strict conversation URL grammar, and share URLs.
 * Only registered-server Chatto routes qualify for navigation in this SPA.
 */
import { resolve } from '$app/paths';
import type { ResolvedPathname } from '$app/types';
import { serverRegistry } from '$lib/client';
import { serverIdToSegment, segmentToServerId } from '$lib/navigation';
import { serverIdForHostname } from '$lib/navigation/serverHostname';

export interface MessageLink {
  /** Null for unknown servers; these links cannot load a preview. */
  serverId: string | null;
  roomId: string;
  threadRootEventId?: string;
  messageId: string;
}

const roomIdPattern = /^(?:R[a-zA-Z0-9]{14}|[a-f0-9]{14})$/;
const eventIdPattern = /^E[a-zA-Z0-9]{14}$/;

/** Parse the shared server-route prefix without consulting the catalogue. */
function parseChatURL(input: string, origin: string) {
  let url: URL;
  let sameOrigin: boolean;
  let parts: string[];
  try {
    url = new URL(input, origin);
    // Compare scheme and host, not `URL.origin`: the desktop app's `chatto:`
    // scheme can have an opaque origin, and its root-relative links must still match.
    const base = new URL(origin);
    sameOrigin = url.protocol === base.protocol && url.host === base.host;
    if (url.username || url.password) return null;
    if (!sameOrigin && !['http:', 'https:'].includes(url.protocol)) return null;
    // Decode each segment separately: encoded slashes must never change the route shape.
    parts = url.pathname.replace(/\/$/, '').split('/').slice(1).map(decodeURIComponent);
  } catch {
    return null;
  }
  const [prefix, serverSegment] = parts;
  if (prefix !== 'chat' || !serverSegment || serverSegment.includes('/')) return null;
  return { url, sameOrigin, serverSegment, parts };
}

/** Strict message grammar used for previews. General routes do not need message IDs. */
function parseChatLink(input: string, origin: string) {
  const parsed = parseChatURL(input, origin);
  if (!parsed) return null;
  const { parts } = parsed;
  const [, , roomId, fourth, fifth, sixth] = parts;
  if (!roomIdPattern.test(roomId)) return null;
  let destination: Omit<MessageLink, 'serverId'>;
  if (parts.length === 5 && fourth === 'm' && eventIdPattern.test(fifth)) {
    destination = { roomId, messageId: fifth };
  } else if (
    parts.length === 6 &&
    eventIdPattern.test(fourth) &&
    fifth === 'm' &&
    eventIdPattern.test(sixth)
  ) {
    destination = { roomId, threadRootEventId: fourth, messageId: sixth };
  } else {
    return null;
  }
  return { ...parsed, destination };
}

/**
 * Rewrite any registered server's /chat/<server>/... route into this frontend.
 * The remaining path, query, and fragment are preserved. Preview parsing stays strict.
 */
export function resolveChatRoute(input: string): ResolvedPathname | null {
  if (typeof window === 'undefined') return null;
  const parsed = parseChatURL(input, window.location.origin);
  if (!parsed) return null;
  const serverId = resolveChatLinkServerId(parsed);
  if (!serverId) return null;
  const prefix = resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverId) });
  const suffix = parsed.url.pathname.split('/').slice(3).join('/');
  const trailingPath = parsed.url.pathname.split('/').length > 3 ? `/${suffix}` : '';
  return `${prefix}${trailingPath}${parsed.url.search}${parsed.url.hash}` as ResolvedPathname;
}

/** Unknown URL origins remain external, even when their path names a known server. */
function resolveChatLinkServerId({
  url,
  sameOrigin,
  serverSegment
}: {
  url: URL;
  sameOrigin: boolean;
  serverSegment: string;
}): string | null {
  if (sameOrigin) return segmentToServerId(serverSegment);
  const originServerId = serverIdForHostname(url.hostname);
  if (!originServerId) return null;
  return serverSegment === '-' ? originServerId : segmentToServerId(serverSegment);
}

/** Build the local message route, including its thread when supplied. */
export function buildMessageLinkPath(
  serverId: string,
  roomId: string,
  messageId: string,
  threadRootEventId?: string | null
): ResolvedPathname {
  const params = { serverId: serverIdToSegment(serverId), roomId, messageId };
  return threadRootEventId
    ? resolve('/chat/[serverId]/[roomId]/[threadId]/m/[messageId]', {
        ...params,
        threadId: threadRootEventId
      })
    : resolve('/chat/[serverId]/[roomId]/m/[messageId]', params);
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

/** Recognize message links for previews, retaining valid links to unknown servers. */
export function parseMessageLink(input: string): MessageLink | null {
  const origin = typeof window !== 'undefined' ? window.location.origin : 'https://_';
  const parsed = parseChatLink(input, origin);
  if (!parsed) return null;
  const serverId = resolveChatLinkServerId(parsed);
  return {
    serverId,
    roomId: parsed.destination.roomId,
    messageId: parsed.destination.messageId,
    threadRootEventId: parsed.destination.threadRootEventId
  };
}
