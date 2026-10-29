/** Serialize parent turns while ordinary child tasks keep a conversation execution alive. */
import { randomUUID } from 'node:crypto';
import { createNotifier, task, Type, type TaskContext } from 'runling';
import type { EventSourceContext, RunHandle } from 'runling/web';
import { Subagents } from './delegate.ts';
import { DEFAULT_INBOX } from './settings.ts';

type Turn = (ctx: TaskContext, children: Subagents) => Promise<void>;
interface Work {
  run: Turn;
  done: ReturnType<typeof Promise.withResolvers<void>>;
  failure?: { error: unknown };
}
interface Owner {
  handle: RunHandle;
  queue: Work[];
  changed: ReturnType<typeof createNotifier>;
  accepting: boolean;
  children?: Subagents;
}

// Only verified source work can create callbacks. Console steering and saved
// inputs cannot reconstruct private work or authorize a reply after restart.
const tickets = new Map<string, Owner>();
const active = new Map<string, Owner>();
const admissions = new WeakMap<EventSourceContext, number>();

/** Ends when parent turns, child work, and completion delivery are all finished. */
export const conversation = task(
  {
    name: 'Botto conversation',
    input: Type.String(),
    result: Type.Null(),
    steering: Type.String()
  },
  async (ctx) => {
    if (ctx.run?.resumed) return null;
    for await (const ticket of ctx.steering) {
      const owner = tickets.get(ticket);
      if (!owner || owner.handle.id !== ctx.run?.id) continue;
      tickets.delete(ticket);
      const children = new Subagents(ctx, () => owner.changed.notify());
      owner.children = children;
      let current: Work | undefined;
      try {
        while (true) {
          ctx.signal.throwIfAborted();
          current = owner.queue.shift();
          if (current) {
            try {
              await current.run(ctx, children);
            } catch (error) {
              current.failure = { error };
              throw new Error('Conversation turn failed');
            }
            ctx.signal.throwIfAborted();
            current.done.resolve();
            current = undefined;
            continue;
          }
          const complete = children.takeCompletion();
          if (complete) {
            await complete();
            continue;
          }
          if (!children.busy) break;
          await owner.changed.wait({ signal: ctx.signal });
        }
      } finally {
        // Stop admission before yielding for cleanup. A racing batch waits for
        // this execution to close, then reactivates the same retained run.
        owner.accepting = false;
        await children.close();
        current?.done.reject(current.failure?.error ?? new Error('Conversation turn failed'));
        for (const work of owner.queue.splice(0))
          work.done.reject(new Error('Conversation closed before the turn completed'));
      }
      return null;
    }
    return null;
  }
);

/** Resolve after this batch, not after its children. Parent transcript access stays serial. */
export async function runTurn(
  source: EventSourceContext,
  key: string,
  callback: Turn,
  maxActive = DEFAULT_INBOX.maxThreads
): Promise<void> {
  source.signal.throwIfAborted();
  const previous = source
    .runs({ includeFinished: true })
    .find((run) => run.workflow === 'bottoConversation' && run.input === key);
  let owner = previous && active.get(previous.id);
  if (!owner?.accepting) {
    await previous?.finished;
    source.signal.throwIfAborted();
    const reserved = admissions.get(source) ?? 0;
    if (
      source.runs().filter((run) => run.workflow === 'bottoConversation').length + reserved >=
      maxActive
    )
      throw new Error('Conversation capacity reached; the message remains queued.');
    let started: RunHandle | undefined;
    admissions.set(source, reserved + 1);
    try {
      await source.dispatch(async (ctx) => {
        started = await ctx.start(conversation, { input: key, runId: previous?.id });
      }, null);
    } finally {
      admissions.set(source, (admissions.get(source) ?? 1) - 1);
    }
    if (!started) throw new Error('Conversation was not registered');
    const handle: RunHandle = started;
    owner = { handle, queue: [], changed: createNotifier(), accepting: true };
    active.set(handle.id, owner);
    const owned = owner;
    const stop = () => handle.cancel();
    source.signal.addEventListener('abort', stop, { once: true });
    if (source.signal.aborted) stop();
    // The subscription belongs to the execution, including time between batches.
    void handle.finished
      .finally(() => {
        owned.accepting = false;
        if (active.get(handle.id) === owned) active.delete(handle.id);
        source.signal.removeEventListener('abort', stop);
        for (const work of owned.queue.splice(0))
          work.done.reject(new Error('Conversation closed before the turn completed'));
      })
      .catch(() => {});
    const ticket = randomUUID();
    tickets.set(ticket, owner);
    // Enqueue before steering: a fast root must not observe an empty queue.
    const work = makeWork(callback);
    owner.queue.push(work);
    try {
      await handle.steer(ticket);
      await work.done.promise;
    } catch (error) {
      handle.cancel();
      await handle.finished;
      throw error;
    } finally {
      tickets.delete(ticket);
      if (!owner.children?.busy) await handle.finished;
    }
    source.signal.throwIfAborted();
    return;
  }
  const work = makeWork(callback);
  owner.queue.push(work);
  owner.changed.notify();
  await work.done.promise;
  if (!owner.children?.busy) await owner.handle.finished;
  source.signal.throwIfAborted();
}

/** Source failures also stop children between batches, before the Chatto client closes. */
export async function closeConversations(source: EventSourceContext): Promise<void> {
  const owned = source.runs().filter((run) => active.has(run.id));
  for (const run of owned) run.cancel();
  await Promise.all(owned.map((run) => run.finished));
}

/** Attach rejection handling before admission; failures settle after root cleanup. */
function makeWork(callback: Turn): Work {
  const done = Promise.withResolvers<void>();
  void done.promise.catch(() => {});
  return { done, run: callback };
}
