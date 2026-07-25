import type { TimelineEventView } from '$lib/render/timelineEvents';

export type EventConnectionPage = {
  events: readonly TimelineEventView[];
  startCursor?: string | null;
  endCursor?: string | null;
  hasOlder: boolean;
  hasNewer: boolean;
};

export function unmask(events: readonly TimelineEventView[]): TimelineEventView[] {
  return events.filter((event): event is TimelineEventView => event !== null);
}

export function getActorId(actor: TimelineEventView['actor']): string | undefined {
  return actor ? (actor as { id?: string }).id : undefined;
}
