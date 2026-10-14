import type { TaskContext } from './context.ts';
import { implementation, invoke, type Invocation } from './invocation.ts';
import {
  isWorkflowSchema,
  parseBoundary,
  type SchemaInput,
  type SchemaOutput,
  type WorkflowSchema
} from './schema.ts';

/** The contract of a task: its name, its input and output, and the messages that it exchanges
 * with its parent while it runs. Without a message schema, a task accepts any message. */
export interface TaskDefinition<
  Input extends WorkflowSchema = WorkflowSchema,
  Output extends WorkflowSchema = WorkflowSchema,
  Receives extends WorkflowSchema | undefined = WorkflowSchema | undefined,
  Emits extends WorkflowSchema | undefined = WorkflowSchema | undefined
> {
  name: string;
  input: Input;
  output: Output;
  /** Messages from the parent, read from `ctx.inbox`. */
  receives?: Receives;
  /** Messages to the parent, sent with `ctx.emit`. */
  emits?: Emits;
}

/** A value of a message schema before validation; `unknown` without a schema. */
export type MessageInput<S> = S extends WorkflowSchema ? SchemaInput<S> : unknown;
/** A value of a message schema after validation; `unknown` without a schema. */
export type MessageOutput<S> = S extends WorkflowSchema ? SchemaOutput<S> : unknown;

/** A task's implementation. It reads messages and emits messages through `ctx`. */
export type TaskImplementation<
  Input extends WorkflowSchema,
  Output extends WorkflowSchema,
  Receives extends WorkflowSchema | undefined,
  Emits extends WorkflowSchema | undefined
> = (
  ctx: TaskContext<MessageOutput<Receives>, MessageInput<Emits>>,
  input: SchemaOutput<Input>
) => SchemaInput<Output> | Promise<SchemaInput<Output>>;

/**
 * A function with schemas. Calling it starts the task and waits for its result; use
 * `ctx.start` to exchange messages with the task while it runs.
 */
export type Task<
  Input extends WorkflowSchema = WorkflowSchema,
  Output extends WorkflowSchema = WorkflowSchema,
  Receives extends WorkflowSchema | undefined = WorkflowSchema | undefined,
  Emits extends WorkflowSchema | undefined = WorkflowSchema | undefined
> = ((ctx: TaskContext<any, any>, input: SchemaInput<Input>) => Promise<SchemaOutput<Output>>) &
  Readonly<TaskDefinition<Input, Output, Receives, Emits>> & {
    /** Internal: validate and run the implementation in a child context. */
    readonly [implementation]: (ctx: TaskContext<any, any>, input: unknown) => Promise<unknown>;
  };

/** Any task, for APIs that accept every task. */
export type AnyTask = Task<any, any, any, any>;
export type TaskInput<T extends AnyTask> = SchemaInput<T['input']>;
export type TaskOutput<T extends AnyTask> = SchemaOutput<T['output']>;
/** A message schema of a task definition without the `undefined` of its optional property.
 * It is `never` when the task has no message schema. */
type DefinedSchema<S> = Exclude<S, undefined>;
/** What a parent sends to a task. `unknown` when the task has no `receives` schema. */
export type TaskReceives<T extends AnyTask> = [DefinedSchema<T['receives']>] extends [never]
  ? unknown
  : MessageInput<DefinedSchema<T['receives']>>;
/** What a parent receives from a task. `unknown` when the task has no `emits` schema. */
export type TaskEmits<T extends AnyTask> = [DefinedSchema<T['emits']>] extends [never]
  ? unknown
  : MessageOutput<DefinedSchema<T['emits']>>;
/** The handle of a started task. */
export type TaskInvocation<T extends AnyTask> = Invocation<
  TaskReceives<T>,
  TaskEmits<T>,
  TaskOutput<T>
>;

/** Define a task: a named function with input and output schemas, and optional message schemas. */
export function task<
  const Input extends WorkflowSchema,
  const Output extends WorkflowSchema,
  const Receives extends WorkflowSchema | undefined = undefined,
  const Emits extends WorkflowSchema | undefined = undefined
>(
  definition: TaskDefinition<Input, Output, Receives, Emits>,
  run: TaskImplementation<Input, Output, Receives, Emits>
): Task<Input, Output, Receives, Emits> {
  const { name } = definition;
  if (typeof name !== 'string' || name.trim() === '') {
    throw new TypeError('Task name must be a non-empty string');
  }
  for (const boundary of ['input', 'output', 'receives', 'emits'] as const) {
    const schema = definition[boundary];
    if (schema === undefined && (boundary === 'receives' || boundary === 'emits')) continue;
    if (!isWorkflowSchema(schema)) {
      throw new TypeError(`Task ${JSON.stringify(name)} ${boundary} schema is invalid`);
    }
  }

  const execute = async (ctx: TaskContext<any, any>, value: unknown) => {
    const input = await parseBoundary(name, 'input', definition.input, value);
    ctx.signal.throwIfAborted();
    const output = await run(ctx, input as SchemaOutput<Input>);
    return parseBoundary(name, 'output', definition.output, output);
  };

  const defined = (ctx: TaskContext<any, any>, input: unknown) =>
    invoke(ctx, defined as unknown as AnyTask, input, { direct: true }).result;

  Object.defineProperties(defined, {
    name: { value: name },
    input: { value: definition.input, enumerable: true },
    output: { value: definition.output, enumerable: true },
    ...(definition.receives === undefined
      ? {}
      : { receives: { value: definition.receives, enumerable: true } }),
    ...(definition.emits === undefined
      ? {}
      : { emits: { value: definition.emits, enumerable: true } }),
    [implementation]: { value: execute }
  });
  return defined as unknown as Task<Input, Output, Receives, Emits>;
}

/** Check whether a value was defined with `task()`. */
export const isTask = (value: unknown): value is AnyTask =>
  typeof value === 'function' &&
  typeof (value as unknown as Record<symbol, unknown>)[implementation] === 'function';
