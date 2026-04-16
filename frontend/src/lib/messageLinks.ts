/**
 * Helpers for building and parsing Chatto message links.
 *
 * A message link has the shape `/chat/<instanceSegment>/<spaceId>/<roomId>/m/<messageId>`.
 * The `m/` prefix distinguishes message URLs from the `[threadId]` route that sits
 * at the same level (thread IDs and message IDs share the same ID space).
 */

import { resolve } from '$app/paths';
import { instanceRegistry } from '$lib/state/instance/registry.svelte';
import { instanceIdToSegment } from '$lib/navigation';

export interface MessageLink {
  /** URL segment for the instance (`-` for origin, hostname for remote). */
  instanceSegment: string;
  /** Resolved instance ID, or null if the segment doesn't match a registered instance. */
  instanceId: string | null;
  spaceId: string;
  roomId: string;
  messageId: string;
}

/**
 * Build the internal path for a message link (no origin).
 */
export function buildMessageLinkPath(
  instanceId: string,
  spaceId: string,
  roomId: string,
  messageId: string
): string {
  return resolve('/chat/[instanceId]/[spaceId]/[roomId]/m/[messageId]', {
    instanceId: instanceIdToSegment(instanceId),
    spaceId,
    roomId,
    messageId
  });
}

/**
 * Build an absolute URL for a message link, including origin. Used when the
 * user copies a shareable link to their clipboard.
 */
export function buildMessageLinkURL(
  instanceId: string,
  spaceId: string,
  roomId: string,
  messageId: string
): string {
  const path = buildMessageLinkPath(instanceId, spaceId, roomId, messageId);

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
  // Expected: ['chat', instanceSegment, spaceId, roomId, 'm', messageId]
  if (parts.length !== 6) return null;
  if (parts[0] !== 'chat' || parts[4] !== 'm') return null;

  const [, instanceSegment, spaceId, roomId, , messageId] = parts;
  const effectiveSegment = hostnameSegment ?? instanceSegment;

  return {
    instanceSegment: effectiveSegment,
    instanceId: resolveInstanceIdForSegment(effectiveSegment),
    spaceId,
    roomId,
    messageId
  };
}

/**
 * Resolve an instance segment against the registry. Accepts both `-` and raw
 * hostnames. Returns null if no registered instance matches.
 */
function resolveInstanceIdForSegment(segment: string): string | null {
  if (segment === '-') {
    return instanceRegistry.originInstance?.id ?? null;
  }

  for (const instance of instanceRegistry.instances) {
    try {
      if (new URL(instance.url).hostname === segment) {
        return instance.id;
      }
    } catch {
      continue;
    }
  }

  return null;
}
