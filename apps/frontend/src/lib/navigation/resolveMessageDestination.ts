/** Resolve message links without coupling user and room route components. */
import { goto } from '$app/navigation';
import type { ResolvedPathname } from '$app/types';
import type { RoomTimelineAPI } from '@chatto/client/api/roomTimeline';
import { TimelineEventKind } from '@chatto/client/timeline/timelineEvents';
import type { PendingHighlightStore } from '$lib/state/server/pendingHighlight';

/**
 * Open the message's thread when present, and deliver its highlight intent.
 * Missing messages retain a room highlight; failed requests return without one.
 * The caller owns the URL and guards against navigation after its scope changes.
 */
export async function resolveAndRedirect(
  api: Pick<RoomTimelineAPI, 'getMessage'>,
  pendingHighlights: PendingHighlightStore,
  destination: (threadId?: string) => ResolvedPathname,
  roomId: string,
  messageId: string,
  isCurrent: () => boolean = () => true
): Promise<void> {
  try {
    const target = await api.getMessage({ roomId, eventId: messageId });
    if (!isCurrent()) return;

    const threadId =
      target?.event.kind === TimelineEventKind.MessagePosted
        ? target.event.threadRootEventId || (target.event.threadExists ? target.id : null)
        : null;
    pendingHighlights.set(roomId, threadId, messageId);
    void goto(destination(threadId ?? undefined), { replaceState: true });
  } catch {
    if (!isCurrent()) return;
    void goto(destination(), { replaceState: true });
  }
}
