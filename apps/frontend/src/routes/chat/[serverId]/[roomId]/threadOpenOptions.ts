import type { QuoteInsertionContent } from '$lib/state/room';

export type PendingThreadReply = {
  eventId: string;
  actorDisplayName: string;
  excerpt: string;
};

export type PendingThreadReplyRequest = PendingThreadReply & {
  id: number;
  threadRootEventId: string;
};

export type ThreadOpenOptions = {
  highlightEventId?: string;
  quoteText?: QuoteInsertionContent;
  reply?: PendingThreadReply;
};

export type OpenThreadHandler = (threadRootEventId: string, options?: ThreadOpenOptions) => void;
