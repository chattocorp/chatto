/** Define the journal events shared by workflows, the server, and the console. */
import { AsyncLocalStorage } from 'node:async_hooks';
import type { TokenUsage } from './usage.ts';
import type { LogFields } from './log-fields.ts';

export type RunlingEventPayload =
  /** Console history boundary; closes unfinished activities before another execution. */
  | { type: 'execution.finished' }
  /** Display metadata for the workflow run, independent of task names and routing. */
  | { type: 'run.title'; title: string }
  /** Links a task invocation's message channel to the step that runs the task. */
  | { type: 'task.linked'; channelId: string; taskId: string }
  | {
      /** A message between a parent and a task. `input` goes to the task, `update` comes from it. */
      type: 'message.sent';
      id: string;
      channelId: string;
      direction: 'input' | 'update';
      payload: string;
    }
  /** The task read the message from its inbox, or the parent read the update. */
  | { type: 'message.read'; id: string }
  /** A task waits for a message from its parent: its inbox is empty. */
  | { type: 'wait.started'; id: string; channelId: string }
  /** The wait ended: a message arrived, or the input closed or failed. */
  | {
      type: 'wait.finished';
      id: string;
      status: 'received' | 'closed';
      durationMs: number;
    }
  | {
      type: 'log';
      level: 'debug' | 'info' | 'success' | 'warn' | 'error';
      message: string;
      /** Host-supplied metadata, without model content or credentials. */
      fields?: LogFields;
      depth: number;
      color: string;
      source?: 'step' | 'agent' | 'command';
      sourceId?: string;
    }
  | {
      type: 'step.started';
      id: string;
      label: string;
    }
  | {
      type: 'step.finished';
      id: string;
      status: 'completed' | 'failed';
      durationMs: number;
      /** Error message of a failed step, at most 2,000 characters. Journal only. */
      error?: string;
    }
  | {
      type: 'command.started';
      id: string;
      command: string;
    }
  | {
      type: 'command.finished';
      id: string;
      status: 'completed' | 'failed';
      durationMs: number;
      output: {
        stdout: string;
        stderr: string;
      };
    }
  | {
      type: 'agent.started';
      agentId: string;
      label?: string;
      model: string;
      color: string;
    }
  | {
      type: 'agent.progress';
      agentId: string;
      text: string;
    }
  | {
      /** Safe tool lifecycle metadata; excludes arguments, paths, and output. */
      type: 'agent.tool';
      agentId: string;
      /** Registered identifier, never tool arguments. */
      toolName?: string;
      operation: 'read' | 'search' | 'edit' | 'command' | 'other';
      phase: 'started' | 'succeeded' | 'failed';
    }
  | {
      type: 'agent.action';
      agentId: string;
      action: string;
    }
  | {
      type: 'agent.usage';
      agentId: string;
      usage: TokenUsage;
    }
  | {
      type: 'agent.finished';
      agentId: string;
      outcome: 'completed' | 'blocked' | 'failed';
      usage: TokenUsage;
    }
  | {
      type: 'usage.updated';
      usage: TokenUsage;
    };

export type RunlingEvent = RunlingEventPayload & {
  activityId?: string;
  timestamp: number;
};

export type RunlingEventListener = (event: RunlingEvent) => void;

const listeners = new AsyncLocalStorage<readonly RunlingEventListener[]>();
const activity = new AsyncLocalStorage<string>();

export const emitRunlingEvent = (payload: RunlingEventPayload): void => {
  const event = {
    ...payload,
    activityId: activity.getStore(),
    timestamp: performance.now()
  } as RunlingEvent;

  for (const listener of listeners.getStore() ?? []) listener(event);
};

export const observeRunlingEvents = <T>(listener: RunlingEventListener, work: () => T): T =>
  listeners.run([...(listeners.getStore() ?? []), listener], work);

/** A host run owns its observers and activity, even when a callback dispatches it from another run. */
export const withRunlingEventScope = <T>(listener: RunlingEventListener, work: () => T): T =>
  listeners.run([listener], () => activity.exit(work));

export const withRunlingActivity = <T>(id: string, work: () => T): T => activity.run(id, work);

/** Return the active task ID for context-bound observations. */
export const currentRunlingActivity = (): string | undefined => activity.getStore();

export const bindRunlingContext = <Args extends unknown[], Result>(
  work: (...args: Args) => Result
): ((...args: Args) => Result) => {
  const capturedListeners = listeners.getStore() ?? [];
  const capturedActivity = activity.getStore();

  return (...args) =>
    listeners.run(capturedListeners, () =>
      capturedActivity === undefined
        ? activity.exit(() => work(...args))
        : activity.run(capturedActivity, () => work(...args))
    );
};
