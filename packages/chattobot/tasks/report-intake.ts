/** Classify an unsolicited report, then own its conversation in the same workflow run. */
import { log, task, Type, type Static } from 'runling';
import { ConversationInput, IncomingMessage } from '../chatto/message.ts';
import type { createConversation } from './conversation.ts';
import type { createIntakeClassifier, IntakeInput } from './classify-intake.ts';

interface IntakeOptions {
  classifier: ReturnType<typeof createIntakeClassifier>;
  conversation: ReturnType<typeof createConversation>;
  /** Read current content and verify its human author. Missing or deleted posts are ignored. */
  read: (
    message: IncomingMessage,
    signal: AbortSignal
  ) => Promise<Static<typeof IntakeInput> | undefined>;
}

/** The router only starts this workflow and delivers messages. Classification, silence, and
 * the child conversation belong here. An addressed message can bypass pending classification. */
export function createReportIntake({ classifier, conversation, read }: IntakeOptions) {
  return task(
    {
      name: 'Report intake',
      input: IncomingMessage,
      output: Type.Null(),
      receives: ConversationInput
    },
    async (ctx, message) => {
      const reader = ctx.inbox[Symbol.asyncIterator]();
      let queued: IteratorResult<IncomingMessage> | undefined;
      const nextMessage = reader.next().then((item) => {
        queued = item;
        return item;
      });
      // Cancellation can fail the inbox while metadata is still being read.
      void nextMessage.catch(() => {});
      let first: IncomingMessage | undefined;
      let usedNextMessage = false;
      {
        const input = await read(message, ctx.signal).catch(() => {
          ctx.signal.throwIfAborted();
          log.error('Report intake could not read the post; no automatic reply sent');
          return undefined;
        });
        if (queued && !queued.done) {
          first = queued.value;
          usedNextMessage = true;
        } else {
          await using classification = input ? ctx.start(classifier, input) : undefined;
          const decision = (classification?.result ?? Promise.resolve({ engage: false })).then(
            (value) => ({ kind: 'decision' as const, engage: value.engage, failed: false }),
            () => {
              ctx.signal.throwIfAborted();
              return { kind: 'decision' as const, engage: false, failed: true };
            }
          );
          const outcome = await Promise.race([
            nextMessage.then(async (item) =>
              item.done ? await decision : { kind: 'addressed' as const, message: item.value }
            ),
            decision
          ]);
          ctx.signal.throwIfAborted();
          if (outcome.kind === 'addressed') {
            first = outcome.message;
            usedNextMessage = true;
          } else {
            if (outcome.failed) log.error('Report intake failed; no automatic reply sent');
            else log.info(outcome.engage ? 'Report intake accepted' : 'Report intake ignored');
            if (outcome.engage && input)
              first = { ...message, body: input.body, trigger: 'intake' };
          }
          // Disposing the classifier cancels it if an addressed message won the race.
        }
      }
      if (!first) return null;

      await using child = ctx.start(conversation, first);
      const finished = child.result.then(() => ({ done: true as const, value: undefined }));
      const forward = async () => {
        let pending = usedNextMessage ? reader.next() : nextMessage;
        while (true) {
          const item = await Promise.race([pending, finished]);
          if (item.done) break;
          await child.send(item.value);
          pending = reader.next();
        }
        child.closeInput();
      };
      await Promise.all([child.result, forward()]);
      return null;
    }
  );
}
