import type { QuoteInsertionContent } from '$lib/state/room';
import type { AccountNameIdentity } from '$lib/render/accountName';

export type PendingThreadReply = {
  eventId: string;
  actorDisplayName: string;
  actorIdentity?: AccountNameIdentity;
  excerpt: string;
};

export type ThreadOpenOptions = {
  highlightEventId?: string;
  quoteText?: QuoteInsertionContent;
  reply?: PendingThreadReply;
};

export type OpenThreadHandler = (threadRootEventId: string, options?: ThreadOpenOptions) => void;
