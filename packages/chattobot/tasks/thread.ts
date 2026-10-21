/** Own a thread's conversation and decide when ordinary replies need another turn. */
import {
  createNotifier,
  log,
  task,
  Type,
  type Static,
  type TaskContext,
  type TaskInvocation
} from 'runling';
import { IncomingMessage, type ConversationInput } from '../chatto/message.ts';
import type { createConversation } from './conversation.ts';
import type { createIntakeClassifier, ReactionInput, ReportInput } from './classify-intake.ts';
import type { ReadFollowUp } from '../chatto/follow-up.ts';
import { messageBatches } from '../harness/message-batches.ts';

/** Reconcile messages missed during disconnects and check inactivity. Realtime replies wake sooner. */
export const FOLLOW_UP_INTERVAL_MS = 5 * 60_000;
/** Empty or ignored checks do not extend this lifetime. */
export const IDLE_MS = 60 * 60_000;

export interface ThreadOptions {
  classifier: ReturnType<typeof createIntakeClassifier>;
  conversation: ReturnType<typeof createConversation>;
  /** Read current content and verify its human author. Missing or deleted posts are ignored. */
  read: (
    message: IncomingMessage,
    signal: AbortSignal
  ) => Promise<Static<typeof ReportInput> | undefined>;
  readFollowUp: ReadFollowUp;
  /** Recent thread history for reaction decisions, read with the bot's permissions. */
  readReactionContext: (
    message: IncomingMessage,
    signal: AbortSignal
  ) => Promise<Static<typeof ReactionInput>['context']>;
  /** Injectable clock interval for lifecycle tests. */
  intervalMs?: number;
  /** Fixed inbox window for all incoming messages, injectable for lifecycle tests. */
  batchMs?: number;
  idleMs?: number;
}

/** Root-owned JSON data. Live channels, tasks, and timers are recreated on entry. */
interface ThreadState {
  first?: IncomingMessage;
  delivered: string[];
  /** Accepted, ignored, and failed reaction checks must not run again after recovery. */
  checkedReactions: string[];
  /** Realtime replies awaiting consideration, even if the bot posts again before the check. */
  pendingFollowUps: string[];
  checked?: string;
  lastActivity: number;
}

/** The router only starts this workflow and delivers messages. Classification, silence, and
 * the child conversation belong here. An addressed message can bypass pending classification. */
export function createThreadWorkflow(
  configure:
    | ThreadOptions
    | ((ctx: TaskContext<IncomingMessage>, message: IncomingMessage) => Promise<ThreadOptions>)
) {
  return task(
    {
      name: 'Chatto thread',
      input: IncomingMessage,
      result: Type.Null(),
      steering: IncomingMessage
    },
    async (ctx, message) => {
      const saved = ctx.state as Partial<ThreadState>;
      const resume = !!saved.delivered || !!ctx.run?.resumed;
      const hadConversation = !!saved.first;
      saved.delivered ??= [message.id];
      saved.pendingFollowUps ??= [];
      saved.checkedReactions ??= [];
      saved.lastActivity ??= Date.now();
      const state = saved as ThreadState;
      const options = typeof configure === 'function' ? await configure(ctx, message) : configure;
      const { classifier, conversation, read } = options;
      let checkingReactions = false;
      // Keep reaction decisions separate: accepting one must not forward unrelated reactions.
      // Explicit messages still pass if a context read or classifier fails.
      const acceptReactions = async (batch: IncomingMessage[]) => {
        const accepted: IncomingMessage[] = [];
        checkingReactions = true;
        try {
          for (const item of batch) {
            if (!item.reaction) {
              accepted.push(item);
              continue;
            }
            if (
              state.checkedReactions.includes(item.id) ||
              (hadConversation && state.delivered.includes(item.id))
            )
              continue;
            try {
              const context = await options.readReactionContext(item, ctx.signal);
              const decision = await classifier(ctx, { context, reaction: item.reaction });
              if (decision.engage) accepted.push(item);
              else log.info('Thread reaction ignored');
            } catch {
              ctx.signal.throwIfAborted();
              log.error('Thread reaction check failed; no reaction response sent');
            }
            state.checkedReactions.push(item.id);
          }
          return accepted;
        } finally {
          checkingReactions = false;
        }
      };
      const inboxLifetime = new AbortController();
      using inboxScope = { [Symbol.dispose]: () => inboxLifetime.abort() };
      const inbox = messageBatches(
        ctx.steering,
        AbortSignal.any([ctx.signal, inboxLifetime.signal]),
        hadConversation ? [] : [message],
        options.batchMs
      );
      const reader = inbox.batches;
      let notify = () => {};
      const consider = (batch: IncomingMessage[]) => {
        let followUp = false;
        for (const item of batch) {
          if (item.trigger !== 'follow-up' || state.delivered.includes(item.id)) continue;
          if (!state.pendingFollowUps.includes(item.id)) state.pendingFollowUps.push(item.id);
          followUp = true;
        }
        if (followUp) notify();
        return batch.filter((item) => !item.trigger);
      };
      // The first turn uses the same window as all later turns. Recovery never
      // queues the saved first message as a new request.
      let opening = hadConversation ? [] : consider((await reader.next()).value ?? []);
      // Follow-up events are notifications, never an addressed request that can bypass intake.
      // Keep one inbox reader, and retain a notification received before the child starts.
      const nextAddressed = async (): Promise<IteratorResult<IncomingMessage[]>> => {
        while (true) {
          const item = await reader.next();
          if (item.done) return item;
          const addressed = consider(item.value);
          if (addressed.length) return { done: false, value: addressed };
        }
      };
      let queued: IteratorResult<IncomingMessage[]> | undefined;
      const nextMessage = nextAddressed().then((item) => {
        queued = item;
        return item;
      });
      // Cancellation can fail the inbox while metadata is still being read.
      void nextMessage.catch(() => {});
      let first: IncomingMessage | undefined =
        saved.first ?? opening[0] ?? (message.trigger === 'intake' ? undefined : message);
      let usedNextMessage = false;
      if (!first) {
        const input = await read(message, ctx.signal).catch(() => {
          ctx.signal.throwIfAborted();
          log.error('Report intake could not read the post; no automatic reply sent');
          return undefined;
        });
        if (inbox.addressedArrived && !queued) queued = await nextMessage;
        if (queued && !queued.done) {
          opening = queued.value;
          first = opening[0];
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
          let outcome = await Promise.race([
            nextMessage.then(async (item) =>
              item.done ? await decision : { kind: 'addressed' as const, message: item.value }
            ),
            decision
          ]);
          ctx.signal.throwIfAborted();
          // An intake decision must not close the workflow while an addressed
          // request already waits in its batching window.
          if (outcome.kind === 'decision' && inbox.addressedArrived) {
            const item = await nextMessage;
            if (!item.done) outcome = { kind: 'addressed', message: item.value };
          }
          if (outcome.kind === 'addressed') {
            opening = outcome.message;
            first = opening[0];
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
      if (first && (!resume || usedNextMessage)) {
        opening = await acceptReactions(opening.length ? opening : [first]);
        first = opening[0];
        // Preserve input received while the opening reaction was being checked.
        while (!first) {
          let next: IteratorResult<IncomingMessage[]>;
          if (!usedNextMessage && inbox.addressedArrived) {
            next = queued ?? (await nextMessage);
            usedNextMessage = true;
          } else if (usedNextMessage && inbox.pending) {
            // The first pending reader is consumed. Read any later window directly,
            // including ordinary replies, so an ignored reaction cannot strand a ping.
            const batch = await reader.next();
            next = batch.done ? batch : { done: false, value: consider(batch.value) };
          } else break;
          if (next.done) break;
          opening = await acceptReactions(next.value);
          first = opening[0];
        }
        // An ignored reaction must not suppress a report whose intake it interrupted.
        if (!first && message.trigger === 'intake') {
          try {
            const input = await read(message, ctx.signal);
            if (input && (await classifier(ctx, input)).engage)
              first = { ...message, body: input.body, trigger: 'intake' };
          } catch {
            ctx.signal.throwIfAborted();
            log.error('Report intake failed; no automatic reply sent');
          }
        }
      }
      if (!first) return null;

      // Omit absent optional fields so the snapshot contains JSON values only.
      state.first = JSON.parse(JSON.stringify(first)) as IncomingMessage;
      if (!state.delivered.includes(first.id)) state.delivered.push(first.id);
      for (const item of opening)
        if (!state.delivered.includes(item.id)) state.delivered.push(item.id);
      const resumeConversation =
        resume && !usedNextMessage && (hadConversation || message.trigger !== 'intake');
      await using child = ctx.start(conversation, {
        ...first,
        ...(opening.length > 1 ? { messages: opening } : {}),
        ...(resumeConversation ? { resume: true } : {})
      });
      const monitor = monitorConversation(
        ctx,
        child,
        first,
        options,
        state,
        () => inbox.pending || checkingReactions
      );
      notify = monitor.notify;
      if (state.pendingFollowUps.length) notify();
      const finished = child.result.then(() => ({ done: true as const, value: undefined }));
      const forward = async () => {
        let pending = usedNextMessage ? nextAddressed() : nextMessage;
        while (true) {
          const item = await Promise.race([pending, finished]);
          if (item.done) break;
          const accepted = await acceptReactions(item.value);
          if (accepted.length) await monitor.send(accepted.length === 1 ? accepted[0]! : accepted);
          pending = nextAddressed();
        }
        monitor.stop();
        child.closeSteering();
      };
      try {
        await Promise.all([child.result, forward(), monitor.done]);
      } finally {
        monitor.stop();
        if (child.status === 'running') child.cancel();
        await monitor.done;
      }
      return null;
    }
  );
}

/** A thread check has no visible effects until classification accepts a batch. One snapshot key skips
 * unchanged replies. Already-delivered messages are context only; the next check reconsiders
 * the remaining human replies since the last bot post when another reply arrives.
 * Direct input or a new turn cancels an obsolete check before it can send a duplicate batch.
 * Realtime notifications wake the loop; a periodic check recovers missed events and expires
 * idle conversations after an hour without delivered messages;
 * active turns and background jobs can finish first. */
function monitorConversation(
  ctx: TaskContext<IncomingMessage, unknown>,
  child: TaskInvocation<ReturnType<typeof createConversation>>,
  first: IncomingMessage,
  { classifier, readFollowUp, intervalMs = FOLLOW_UP_INTERVAL_MS, idleMs = IDLE_MS }: ThreadOptions,
  saved: ThreadState,
  hasPendingInput: () => boolean
) {
  const lifetime = new AbortController();
  const signal = AbortSignal.any([ctx.signal, lifetime.signal]);
  let state: 'busy' | 'waiting' | 'idle' = 'busy';
  let changed = false;
  const wake = createNotifier();
  const notify = () => {
    changed = true;
    if (state !== 'busy') wake.notify();
  };
  let check: AbortController | undefined;
  let classification: TaskInvocation<ReturnType<typeof createIntakeClassifier>> | undefined;
  const interrupt = () => {
    state = 'busy';
    check?.abort();
    classification?.cancel();
  };
  const stop = () => {
    lifetime.abort();
    interrupt();
  };
  const send = async (message: ConversationInput) => {
    // A turn can finish without a bot post. The persisted IDs deduplicate delivery.
    const fresh = (Array.isArray(message) ? message : [message]).filter(
      (item) => !saved.delivered.includes(item.id)
    );
    if (!fresh.length) return;
    interrupt();
    await child.steer(Array.isArray(message) ? fresh : fresh[0]!);
    for (const item of fresh) saved.delivered.push(item.id);
    saved.pendingFollowUps = saved.pendingFollowUps.filter((id) => !saved.delivered.includes(id));
    saved.lastActivity = Date.now();
  };
  const updates = (async () => {
    try {
      for await (const update of child.output) {
        if (update === 'busy') interrupt();
        else {
          state = update;
          if (changed) wake.notify();
        }
      }
    } finally {
      stop();
    }
  })();
  const polls = (async () => {
    while (!signal.aborted) {
      let considered: readonly string[] | undefined;
      let key: string | undefined;
      let accepted: IncomingMessage[] | undefined;
      try {
        await wake.wait({ signal, timeoutMs: intervalMs });
        signal.throwIfAborted();
        if (state === 'busy' || hasPendingInput()) continue;
        if (state === 'idle' && Date.now() - saved.lastActivity >= idleMs) {
          stop();
          child.closeSteering();
          break;
        }
        changed = false;
        check = new AbortController();
        const checkSignal = AbortSignal.any([signal, check.signal]);
        const pendingIds = [...saved.pendingFollowUps];
        const snapshot = await readFollowUp(first, checkSignal, pendingIds);
        checkSignal.throwIfAborted();
        // A reply can arrive while the read is in flight. Do not classify it
        // through reconciliation before its inbox window has ended.
        if (hasPendingInput()) {
          changed = true;
          continue;
        }
        considered = pendingIds;
        if (!snapshot) continue;
        const pending = snapshot.messages.filter(
          ({ message }) => !saved.delivered.includes(message.id)
        );
        if (!pending.length) continue;
        const messages = pending.map(({ message }) => message);
        key = JSON.stringify([snapshot.boundary, messages.map((item) => item.id)]);
        if (key === saved.checked) continue;
        {
          await using decision = ctx.start(classifier, {
            ...snapshot.input,
            context: [
              ...snapshot.input.context,
              ...snapshot.messages
                .filter(({ message }) => saved.delivered.includes(message.id))
                .map(({ message }) => ({ role: 'human' as const, body: message.body }))
            ],
            messages: pending.map(({ message, attachmentTypes }) => ({
              body: message.body,
              attachmentTypes
            }))
          });
          classification = decision;
          const { engage } = await decision.result;
          checkSignal.throwIfAborted();
          if (engage) accepted = messages;
          else log.info('Thread follow-up ignored');
        }
      } catch {
        if (!signal.aborted && !check?.signal.aborted)
          log.error('Thread follow-up check failed; no automatic reply sent');
      } finally {
        // A completed or failed decision consumes this snapshot, not notifications that
        // arrived during it. Cancellation by a direct turn leaves pending IDs for the next
        // idle check. Finish this check before forwarding an accepted batch.
        if (!signal.aborted && considered && !check?.signal.aborted) {
          const consumed = new Set(considered);
          saved.pendingFollowUps = saved.pendingFollowUps.filter((id) => !consumed.has(id));
          if (key) saved.checked = key;
        } else if (!signal.aborted && check?.signal.aborted && saved.pendingFollowUps.length) {
          notify();
        }
        if (signal.aborted || check?.signal.aborted) accepted = undefined;
        classification = undefined;
        check = undefined;
      }
      if (accepted) {
        await send(accepted);
        log.info('Thread follow-up accepted');
      }
    }
  })();
  return { send, notify, stop, done: Promise.all([updates, polls]).then(() => {}) };
}
