/** Define the journal events shared by workflows, the server, and the console. */
import { AsyncLocalStorage } from 'node:async_hooks';
import type { TokenUsage } from './usage.ts';
import type { JsonValue } from './runtime.ts';

export type RunlingEventPayload =
  | { type: 'workflow.resumed'; attempt: number }
  | { type: 'conversation.started' }
  | { type: 'task.linked'; channelId: string; taskId: string }
  | { type: 'task.state'; taskId: string; state: { [key: string]: JsonValue } }
  | {
      type: 'task.activity';
      channelId: string;
      message: string;
      level?: 'info' | 'success' | 'error';
    }
  | {
      type: 'message.sent';
      id: string;
      channelId: string;
      direction: 'input' | 'update';
      payload: string;
    }
  | { type: 'message.read'; id: string }
  | { type: 'message.receipt'; id: string; consumed: boolean }
  | {
      type: 'log';
      level: 'debug' | 'info' | 'success' | 'error';
      message: string;
      depth: number;
      color: string;
      source?: 'step' | 'agent' | 'command' | 'input';
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
      type: 'input.requested';
      id: string;
      message: string;
      defaultValue?: string;
    }
  | {
      type: 'input.finished';
      id: string;
      status: 'answered';
      value: string;
      durationMs: number;
    }
  | {
      type: 'input.finished';
      id: string;
      status: 'failed';
      reason?: 'timeout' | 'cancelled';
      durationMs: number;
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
        ? work(...args)
        : activity.run(capturedActivity, () => work(...args))
    );
};
