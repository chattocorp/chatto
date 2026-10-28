/** Agent task definitions share the normal task lifecycle. Each invocation owns its session. */
import { Type, type TString } from 'typebox';
import { scheduler } from 'node:timers/promises';
import {
  AgentOutcomeError,
  createAgentSession,
  type AgentOptions,
  type RunlingAgent
} from './agent.ts';
import type { TaskContext } from './context.ts';
import type { SchemaOutput, WorkflowSchema } from './schema.ts';
import { task, type Task } from './task.ts';

/** Session configuration for an agent task. Agent tasks use text output. */
export type AgentTaskSessionOptions = Omit<AgentOptions, 'output'>;

/** A session factory, also useful for host integrations and tests. */
export type AgentSessionFactory = (options: AgentOptions) => Promise<RunlingAgent>;

type SessionConfiguration<Input extends WorkflowSchema> =
  | (AgentTaskSessionOptions & { setup?: never })
  | {
      /** Resolve credentials, tools, working directories, and session paths per invocation.
       * Called after input validation. Use ctx.signal for cancellable setup work. */
      setup: (
        ctx: TaskContext<string, string>,
        input: SchemaOutput<Input>
      ) => AgentTaskSessionOptions | Promise<AgentTaskSessionOptions>;
    };

/** Define an agent task with typed input and a function that turns it into a prompt. */
export type AgentTaskOptions<Input extends WorkflowSchema> = SessionConfiguration<Input> & {
  name: string;
  input: Input;
  prompt: (input: SchemaOutput<Input>) => string | Promise<string>;
};

/** The common agent task accepts a string directly as its prompt. */
export type TextAgentTaskOptions = SessionConfiguration<TString> & {
  name: string;
  input?: never;
  prompt?: (input: string) => string | Promise<string>;
};

/** Define a reusable agent task. It returns its final text, emits completed replies, and accepts
 * string steering messages. Each invocation creates and disposes one session. Messages that
 * cannot enter the active turn become subsequent turns in that session. The task finishes when
 * its work ends; it does not wait for the parent to close its input. A sessionFile explicitly
 * continues saved history; use a distinct file per concurrent invocation. */
export function agent<const Input extends WorkflowSchema>(
  options: AgentTaskOptions<Input>,
  createSession?: AgentSessionFactory
): Task<Input, TString, TString, TString>;
export function agent(
  options: TextAgentTaskOptions,
  createSession?: AgentSessionFactory
): Task<TString, TString, TString, TString>;
export function agent<const Input extends WorkflowSchema>(
  options: AgentTaskOptions<Input> | TextAgentTaskOptions,
  createSession: AgentSessionFactory = createAgentSession
) {
  if (hasTypedInput(options)) return defineAgentTask(options, createSession);
  return defineAgentTask<TString>(
    { ...options, input: Type.String(), prompt: options.prompt ?? ((text) => text) },
    createSession
  );
}

function hasTypedInput<Input extends WorkflowSchema>(
  options: AgentTaskOptions<Input> | TextAgentTaskOptions
): options is AgentTaskOptions<Input> {
  return options.input !== undefined;
}

function defineAgentTask<Input extends WorkflowSchema>(
  options: AgentTaskOptions<Input>,
  createSession: AgentSessionFactory
) {
  return task(
    {
      name: options.name,
      input: options.input,
      result: Type.String(),
      steering: Type.String(),
      output: Type.String()
    },
    async (ctx, input) => {
      const settings = options.setup ? await options.setup(ctx, input) : options;
      ctx.signal.throwIfAborted();
      const prompt = await options.prompt(input);
      if (typeof prompt !== 'string') throw new TypeError('An agent prompt must be a string');
      ctx.signal.throwIfAborted();
      const session = await createSession({ ...settings, output: 'text' });
      let disposal: Promise<void> | undefined;
      const dispose = () => (disposal ??= Promise.resolve(session[Symbol.asyncDispose]()));
      // A native tool can wait for its transport despite model cancellation.
      // Close transports immediately, then join disposal before the task returns.
      const abort = () => {
        void dispose().catch(() => {});
      };
      ctx.signal.addEventListener('abort', abort, { once: true });
      try {
        ctx.signal.throwIfAborted();
        return await converse(ctx, session, prompt);
      } finally {
        ctx.signal.removeEventListener('abort', abort);
        await dispose();
      }
    }
  );
}

/** Forward replies and steering without making the session wait for its parent's open inbox.
 * An update delivery error cancels the active turn and fails the task, as for other tasks. */
async function converse(ctx: TaskContext<string, string>, session: RunlingAgent, prompt: string) {
  const controller = new AbortController();
  const signal = AbortSignal.any([ctx.signal, controller.signal]);
  const reader = ctx.steering[Symbol.asyncIterator]();
  const stop = Promise.withResolvers<void>();
  const pending: string[] = [];
  const steering: { text: string; delivered: Promise<boolean> }[] = [];
  let updates = Promise.resolve();
  let finished = false;
  const fail = (error: unknown) => controller.abort(error);
  const onText = (text: string) => {
    updates = updates.then(() => ctx.emit(text));
    void updates.catch(fail);
  };

  // Start the first turn before reading the inbox, so initial steering sees a running session.
  let turn = session.runOutcome(ctx, prompt, { signal, onText });
  const receive = (async () => {
    while (!finished) {
      const next = await Promise.race([reader.next(), stop.promise.then(() => undefined)]);
      if (!next || next.done) return;
      const delivered = session.steer(next.value);
      steering.push({ text: next.value, delivered });
      void delivered.catch(fail);
    }
  })();
  void receive.catch(fail);

  try {
    while (true) {
      const result = await turn;
      await updates;
      // Inbox reads pass through asynchronous validation and journal delivery. Give queued
      // messages a scheduling turn to reach the pump before deciding that the task is done.
      // This also covers a fast model turn that ends before its first inbox read settles.
      await scheduler.yield();
      while (steering.length) {
        const batch = steering.splice(0);
        const delivered = await Promise.all(batch.map((item) => item.delivered));
        for (const [index, item] of batch.entries()) if (!delivered[index]) pending.push(item.text);
      }
      signal.throwIfAborted();
      if (result.outcome !== 'completed') throw new AgentOutcomeError(result);
      const next = pending.shift();
      if (next === undefined) return result.summary;
      turn = session.runOutcome(ctx, next, { signal, onText });
    }
  } finally {
    finished = true;
    stop.resolve();
    await reader.return?.();
    await receive;
  }
}
