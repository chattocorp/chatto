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
import {
  reactionEmoji,
  type IntakeDecision,
  type createIntakeClassifier,
  type ReactionInput,
  type ReportInput
} from './classify-intake.ts';
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
  /** Recent thread history, excluding new text being classified, read with the bot's permissions. */
  readContext: (
    messages: IncomingMessage[],
    signal: AbortSignal
  ) => Promise<Static<typeof ReactionInput>['context']>;
  /** Apply one bounded emoji to a verified text message, never an event ID. */
  react: (message: IncomingMessage, emoji: string, signal: AbortSignal) => Promise<void>;
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
  /** Legacy snapshots stored consumed reaction events separately. */
  checkedReactions?: string[];
  /** Inputs consumed without an agent turn, including uncertain reaction writes. */
  consumed: string[];
  /** Realtime replies awaiting consideration, even if the bot posts again before the check. */
  pendingFollowUps: string[];
  checked?: string;
  lastActivity: number;
}

/** The router only starts this workflow and delivers messages. Classification, silence, and
 * the child conversation belong here. Addressed input interrupts an obsolete check and receives its own classification. */
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
      saved.delivered ??= [];
      saved.pendingFollowUps ??= [];
      saved.consumed ??= saved.checkedReactions ?? [];
      delete saved.checkedReactions;
      saved.lastActivity ??= Date.now();
      const state = saved as ThreadState;
      const options = typeof configure === 'function' ? await configure(ctx, message) : configure;
      const { classifier, conversation, read } = options;
      let checkingInput = false;
      const applyDecision = async (
        decision: IntakeDecision,
        messages: IncomingMessage[],
        signal: AbortSignal
      ) => {
        if (decision.action === 'engage') return messages;
        if (decision.action === 'ignore') return [];
        for (const item of messages)
          if (!state.consumed.includes(item.id)) state.consumed.push(item.id);
        const target = messages.at(-1);
        if (target && !target.reaction) {
          // Consume before the write: an uncertain acknowledgement must not repeat it.
          try {
            signal.throwIfAborted();
            await options.react(target, reactionEmoji[decision.action], signal);
          } catch {
            signal.throwIfAborted();
            log.error('Participation reaction failed; no automatic reply sent');
          }
        }
        return [];
      };
      // Explicit text and incoming reactions share one gate. Reactions stay separate so
      // approval of one proposal cannot admit an unrelated reaction in the same window.
      const accept = async (batch: IncomingMessage[]) => {
        checkingInput = true;
        try {
          const fresh = batch.filter(
            (item, index) =>
              batch.findIndex((other) => other.id === item.id) === index &&
              !state.consumed.includes(item.id) &&
              !state.delivered.includes(item.id)
          );
          const accepted = fresh.filter((item) => item.trigger === 'intake');
          const text = fresh.filter((item) => !item.reaction && !item.trigger);
          if (text.length) {
            const verified: IncomingMessage[] = [];
            try {
              const inputs = [];
              for (const item of text) {
                const input = await read(item, ctx.signal);
                if (input) {
                  verified.push({ ...item, body: input.body });
                  inputs.push(input);
                }
              }
              if (verified.length) {
                const context = await options.readContext(verified, ctx.signal);
                const decision = await classifier(ctx, {
                  addressed: true,
                  trackHandled: state.first?.trigger === 'intake',
                  context,
                  messages: inputs
                });
                if (decision.action === 'ignore')
                  state.consumed.push(...verified.map((item) => item.id));
                accepted.push(...(await applyDecision(decision, verified, ctx.signal)));
              }
            } catch {
              ctx.signal.throwIfAborted();
              // A model/context outage must not swallow verified explicit requests.
              log.error('Addressed participation check failed; forwarding verified messages');
              accepted.push(...verified);
            }
          }
          for (const item of fresh.filter((item) => item.reaction)) {
            try {
              const context = await options.readContext([item], ctx.signal);
              const decision = await classifier(ctx, { context, reaction: item.reaction! });
              if (decision.action === 'engage') accepted.push(item);
            } catch {
              ctx.signal.throwIfAborted();
              log.error('Thread reaction check failed; no reaction response sent');
            }
            state.consumed.push(item.id);
          }
          // Preserve input order and authenticated provenance in mixed batches.
          return accepted.sort(
            (a, b) =>
              batch.findIndex((item) => item.id === a.id) -
              batch.findIndex((item) => item.id === b.id)
          );
        } finally {
          checkingInput = false;
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
          if (
            item.trigger !== 'follow-up' ||
            state.delivered.includes(item.id) ||
            state.consumed.includes(item.id)
          )
            continue;
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
      let queued: IteratorResult<IncomingMessage[]> | undefined;
      const nextAddressed = async (): Promise<IteratorResult<IncomingMessage[]>> => {
        while (true) {
          const item = await reader.next();
          if (item.done) return (queued = item);
          const addressed = consider(item.value);
          if (addressed.length) return (queued = { done: false, value: addressed });
        }
      };
      let nextMessage = nextAddressed();
      // Cancellation can fail the inbox while metadata is still being read.
      void nextMessage.catch(() => {});
      let first: IncomingMessage | undefined =
        saved.first ?? opening[0] ?? (message.trigger === 'intake' ? undefined : message);
      let usedNextMessage = false;
      const advance = () => {
        queued = undefined;
        usedNextMessage = false;
        nextMessage = nextAddressed();
        void nextMessage.catch(() => {});
      };
      const addressedPending = () => inbox.addressedPending || (queued && !queued.done);
      // Each retry uses the same race with addressed input. Ignoring a reaction must
      // neither suppress the original report nor strand a ping during its next check.
      while (true) {
        if (!first) {
          const input = await read(message, ctx.signal).catch(() => {
            ctx.signal.throwIfAborted();
            log.error('Report intake could not read the post; no automatic reply sent');
            return undefined;
          });
          if (addressedPending() && !queued) queued = await nextMessage;
          if (queued && !queued.done) {
            opening = queued.value;
            first = opening[0];
            usedNextMessage = true;
          } else {
            await using classification = input ? ctx.start(classifier, input) : undefined;
            const decision = (
              classification?.result ?? Promise.resolve({ action: 'ignore' as const })
            ).then(
              (value) => ({ kind: 'decision' as const, decision: value, failed: false }),
              () => {
                ctx.signal.throwIfAborted();
                return {
                  kind: 'decision' as const,
                  decision: { action: 'ignore' as const },
                  failed: true
                };
              }
            );
            let outcome = await Promise.race([
              nextMessage.then(async (item) =>
                item.done ? await decision : { kind: 'addressed' as const, message: item.value }
              ),
              decision
            ]);
            ctx.signal.throwIfAborted();
            // A negative decision must wait for an addressed message's unfinished window.
            if (outcome.kind === 'decision' && addressedPending()) {
              const item = await nextMessage;
              if (!item.done) outcome = { kind: 'addressed', message: item.value };
            }
            if (outcome.kind === 'addressed') {
              opening = outcome.message;
              first = opening[0];
              usedNextMessage = true;
            } else {
              if (outcome.failed) log.error('Report intake failed; no automatic reply sent');
              else
                log.info(
                  outcome.decision.action === 'engage'
                    ? 'Report intake accepted'
                    : 'Report intake ignored'
                );
              if (input) {
                const accepted = await applyDecision(
                  outcome.decision,
                  [{ ...message, body: input.body, trigger: 'intake' }],
                  ctx.signal
                );
                first = accepted[0];
              }
            }
            // Disposing the classifier cancels it if addressed input won the race.
          }
        }
        if (!first || (resume && !usedNextMessage)) break;
        opening = await accept(opening.length ? opening : [first]);
        first = opening[0];
        // Consume any later addressed windows before allowing a rejected opening to exit.
        while (!first) {
          if (usedNextMessage) advance();
          if (!addressedPending()) break;
          const next = await nextMessage;
          usedNextMessage = true;
          if (next.done) break;
          opening = await accept(next.value);
          first = opening[0];
        }
        if (first || message.trigger !== 'intake') break;
        // The opening contained only ignored reactions. Retry intake without a second
        // batching delay, while the same reader keeps listening for explicit messages.
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
        () => inbox.pending || checkingInput,
        applyDecision
      );
      notify = monitor.notify;
      if (state.pendingFollowUps.length) notify();
      const finished = child.result.then(() => ({ done: true as const, value: undefined }));
      const forward = async () => {
        let pending = usedNextMessage ? nextAddressed() : nextMessage;
        while (true) {
          const item = await Promise.race([pending, finished]);
          if (item.done) break;
          monitor.cancelCheck();
          const accepted = await accept(item.value);
          if (accepted.length) await monitor.send(accepted.length === 1 ? accepted[0]! : accepted);
          else if (state.pendingFollowUps.length) notify();
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
  hasPendingInput: () => boolean,
  decide: (
    decision: IntakeDecision,
    messages: IncomingMessage[],
    signal: AbortSignal
  ) => Promise<IncomingMessage[]>
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
  const cancelCheck = () => {
    check?.abort();
    classification?.cancel();
  };
  const interrupt = () => {
    state = 'busy';
    cancelCheck();
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
          ({ message }) =>
            !saved.delivered.includes(message.id) && !saved.consumed.includes(message.id)
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
                .filter(
                  ({ message }) =>
                    saved.delivered.includes(message.id) || saved.consumed.includes(message.id)
                )
                .map(({ message }) => ({ role: 'human' as const, body: message.body }))
            ],
            messages: pending.map(({ message, attachmentTypes }) => ({
              body: message.body,
              attachmentTypes
            }))
          });
          classification = decision;
          const result = await decision.result;
          checkSignal.throwIfAborted();
          if (hasPendingInput()) {
            check.abort();
            continue;
          }
          accepted = await decide(result, messages, checkSignal);
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
      if (accepted?.length) {
        await send(accepted);
        log.info('Thread follow-up accepted');
      }
    }
  })();
  return { send, notify, stop, cancelCheck, done: Promise.all([updates, polls]).then(() => {}) };
}
