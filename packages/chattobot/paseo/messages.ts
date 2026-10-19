/** Assemble Paseo assistant chunks without polling history or forwarding reasoning/tools. */
import type { PaseoAgentTimelineEvent } from '@getpaseo/client';

/** Maximum retained text per unfinished or emitted assistant segment. */
export const PASEO_MESSAGE_LIMIT = 4000;

/** Collect adjacent assistant chunks. Boundaries emit the previous segment once.
 * The owner must flush when monitoring ends; a segment ending is not proof of success. */
export function createMessageAssembler(emit: (text: string) => void) {
  let pending: { text: string; messageId?: string; turnId?: string } | undefined;
  const flush = () => {
    const text = pending?.text;
    pending = undefined;
    if (text?.trim()) emit(text);
  };
  return {
    flush,
    accept(event: PaseoAgentTimelineEvent['event']) {
      if (event.type === 'timeline' && event.item.type === 'assistant_message') {
        if (
          pending &&
          (pending.messageId !== event.item.messageId || pending.turnId !== event.turnId)
        )
          flush();
        pending = {
          messageId: event.item.messageId,
          turnId: event.turnId,
          text: ((pending?.text ?? '') + event.item.text).slice(-PASEO_MESSAGE_LIMIT)
        };
      } else if (
        event.type === 'timeline' ||
        event.type === 'turn_started' ||
        event.type === 'turn_completed' ||
        event.type === 'turn_failed' ||
        event.type === 'turn_canceled' ||
        event.type === 'permission_requested' ||
        event.type === 'attention_required' ||
        event.type === 'error' ||
        event.type === 'subscription_restored' ||
        event.type === 'replacement'
      ) {
        flush();
      }
    }
  };
}
