/** Route each batch to one finite root execution. Private work is never replayed. */
import { randomUUID } from 'node:crypto';
import { task, Type, type TaskContext } from 'runling';
import type { EventSourceContext, RunHandle } from 'runling/web';

interface Work {
  runId: string;
  run(ctx: TaskContext): Promise<void>;
  completed: boolean;
  failure?: { error: unknown };
}

// Only verified source work gets a private ticket. Neither console messages nor
// saved inputs can recreate callbacks after restart.
const pending = new Map<string, Work>();

/** One execution per batch, retaining the thread's workflow identity across batches. */
export const conversation = task(
  {
    name: 'Botto conversation',
    input: Type.String(),
    result: Type.Null(),
    steering: Type.String()
  },
  async (ctx) => {
    // Recovery of old waiting roots or interrupted batches must not repeat a reply.
    if (ctx.run?.resumed) return null;
    for await (const ticket of ctx.steering) {
      const work = pending.get(ticket);
      if (!work || work.runId !== ctx.run?.id) continue;
      pending.delete(ticket);
      try {
        await work.run(ctx);
        ctx.signal.throwIfAborted();
        work.completed = true;
      } catch (error) {
        work.failure = { error };
        throw new Error('Conversation turn failed');
      }
      return null;
    }
    return null;
  }
);

/** Inbox serializes each thread. The router explicitly selects a retained run or starts one. */
export async function runTurn(
  source: EventSourceContext,
  key: string,
  callback: (ctx: TaskContext) => Promise<void>
): Promise<void> {
  source.signal.throwIfAborted();
  const previous = source
    .runs({ includeFinished: true })
    .find((run) => run.workflow === 'bottoConversation' && run.input === key);
  let run: RunHandle | undefined;
  await source.dispatch(async (ctx) => {
    run = await ctx.start(conversation, { input: key, runId: previous?.id });
  }, null);
  if (!run) throw new Error('Conversation was not registered');
  const handle: RunHandle = run;
  const ticket = randomUUID();
  const work: Work = { runId: handle.id, run: callback, completed: false };
  pending.set(ticket, work);
  const abort = () => handle.cancel();
  source.signal.addEventListener('abort', abort, { once: true });
  try {
    if (source.signal.aborted) abort();
    await handle.steer(ticket);
    await handle.finished;
    if (work.failure) throw work.failure.error;
    source.signal.throwIfAborted();
    if (!work.completed) throw new Error('Conversation closed before the turn completed');
  } finally {
    handle.cancel();
    await handle.finished;
    source.signal.removeEventListener('abort', abort);
    pending.delete(ticket);
  }
}
