import type { QuoteInsertionContent } from '$lib/state/room';
import type { AccountNameIdentity } from '@chatto/client/timeline/accountName';

export type PendingThreadReply = {
  eventId: string;
  actorDisplayName: string;
  actorIdentity?: AccountNameIdentity;
  excerpt: string;
};

export type ThreadOpenOptions = {
  highlightEventId?: string;
  quoteText?: QuoteInsertionContent;
  /** Focus the thread composer without selecting a reply target. */
  focusComposer?: boolean;
  reply?: PendingThreadReply;
};

export type OpenThreadHandler = (threadRootEventId: string, options?: ThreadOpenOptions) => void;
