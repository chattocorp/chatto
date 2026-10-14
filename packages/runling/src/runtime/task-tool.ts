import type { AgentToolResult, ToolDefinition } from '@earendil-works/pi-coding-agent';
import type { TSchema } from 'typebox';
import type { TaskContext } from './context.ts';
import { isStandardSchema } from './schema.ts';
import type { AnyTask } from './task.ts';

export interface TaskToolOptions {
  /** The tool name that the model calls, for example `investigate`. */
  name: string;
  /** Tells the model when to call the tool. */
  description: string;
  /** A short label for logs. Defaults to the task name. */
  label?: string;
}

/**
 * Give a task to an agent as a tool. The tool's parameters are the task's input schema, which
 * must be a TypeBox schema. Each call starts the task as a child of `ctx` and waits for its
 * output, which the model receives as text: a string as it is, other values as JSON. The task's
 * updates are partial tool results. Cancelling the tool call cancels the task.
 */
export function taskTool(
  ctx: TaskContext<any, any>,
  task: AnyTask,
  { name, description, label = task.name }: TaskToolOptions
): ToolDefinition {
  if (isStandardSchema(task.input)) {
    throw new TypeError(
      `Task ${JSON.stringify(task.name)} needs a TypeBox input schema for a tool`
    );
  }
  return {
    name,
    label,
    description,
    parameters: task.input as TSchema,
    async execute(_id, input, signal, onUpdate) {
      const toolCtx = signal ? { ...ctx, signal: AbortSignal.any([ctx.signal, signal]) } : ctx;
      const invocation = toolCtx.start(task, input);
      invocation.closeInput();
      const updates = (async () => {
        for await (const update of invocation.updates) onUpdate?.(textResult(update));
      })();
      // A failed task also fails its updates; its result reports the failure.
      void updates.catch(() => {});
      const output = await invocation.result;
      await updates;
      return textResult(output);
    }
  };
}

const textResult = (value: unknown): AgentToolResult<undefined> => ({
  content: [
    {
      type: 'text',
      text: typeof value === 'string' ? value : (JSON.stringify(value) ?? String(value))
    }
  ],
  details: undefined
});
