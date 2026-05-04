/**
 * Message link URL format: `/chat/<instanceSegment>/<roomId>/m/<messageId>`.
 * The `m/` prefix distinguishes message URLs from the `[threadId]` route that sits
 * at the same level (thread IDs and message IDs share the same ID space).
 */

import { resolve } from '$app/paths';
import { instanceRegistry } from '$lib/state/instance/registry.svelte';
import { instanceIdToSegment, segmentToInstanceId } from '$lib/navigation';

export interface MessageLink {
  /** URL segment for the instance (`-` for origin, hostname for remote). */
  instanceSegment: string;
  /** Resolved instance ID, or null if the segment doesn't match a registered instance. */
  instanceId: string | null;
  roomId: string;
  messageId: string;
}

export function buildMessageLinkPath(
  instanceId: string,
  roomId: string,
  messageId: string
): string {
  return resolve('/chat/[instanceId]/(chrome)/[roomId]/m/[messageId]', {
    instanceId: instanceIdToSegment(instanceId),
    roomId,
    messageId
  });
}

/** Absolute URL for clipboard copy. */
export function buildMessageLinkURL(
  instanceId: string,
  roomId: string,
  messageId: string
): string {
  const path = buildMessageLinkPath(instanceId, roomId, messageId);

  const instance = instanceRegistry.getInstance(instanceId);
  if (instance) {
    try {
      return new URL(path, instance.url).toString();
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
 * Resolves the instance segment against the registry when possible so the
 * caller can tell whether the link points at a known (reachable) instance.
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
  // Expected: ['chat', instanceSegment, roomId, 'm', messageId]
  if (parts.length !== 5) return null;
  if (parts[0] !== 'chat' || parts[3] !== 'm') return null;

  const [, instanceSegment, roomId, , messageId] = parts;
  const effectiveSegment = hostnameSegment ?? instanceSegment;

  return {
    instanceSegment: effectiveSegment,
    instanceId: segmentToInstanceId(effectiveSegment),
    roomId,
    messageId
  };
}
