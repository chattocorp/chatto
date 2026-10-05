import type { TimelineEventView } from '../../timeline/timelineEvents.js';

export function unmask(events: readonly TimelineEventView[]): TimelineEventView[] {
  return events.filter((event): event is TimelineEventView => event !== null);
}

export function getActorId(actor: TimelineEventView['actor']): string | undefined {
  return actor ? (actor as { id?: string }).id : undefined;
}
