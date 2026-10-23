import type { TaskContext } from './context.ts';
import { implementation, invoke, type Invocation } from './invocation.ts';
import {
  isWorkflowSchema,
  parseBoundary,
  type SchemaInput,
  type SchemaOutput,
  type WorkflowSchema
} from './schema.ts';

/** The contract of a task: its name, its input and result, and the messages that it exchanges
 * with its parent while it runs. Without a message schema, a task accepts any message. */
export interface TaskDefinition<
  Input extends WorkflowSchema = WorkflowSchema,
  Result extends WorkflowSchema = WorkflowSchema,
  Steering extends WorkflowSchema | undefined = WorkflowSchema | undefined,
  Output extends WorkflowSchema | undefined = WorkflowSchema | undefined
> {
  name: string;
  /** Initial argument, validated before the implementation starts. */
  input: Input;
  /** Final return value, separate from streamed output. */
  result: Result;
  /** Messages from the parent, read from `ctx.steering`. */
  steering?: Steering;
  /** Messages to the parent, sent with `ctx.emit`. */
  output?: Output;
}

/** A value of a message schema before validation; `unknown` without a schema. */
export type MessageInput<S> = S extends WorkflowSchema ? SchemaInput<S> : unknown;
/** A value of a message schema after validation; `unknown` without a schema. */
export type MessageOutput<S> = S extends WorkflowSchema ? SchemaOutput<S> : unknown;

/** A task implementation consumes steering, emits output, and returns its final result. */
export type TaskImplementation<
  Input extends WorkflowSchema,
  Result extends WorkflowSchema,
  Steering extends WorkflowSchema | undefined,
  Output extends WorkflowSchema | undefined
> = (
  ctx: TaskContext<MessageOutput<Steering>, MessageInput<Output>>,
  input: SchemaOutput<Input>
) => SchemaInput<Result> | Promise<SchemaInput<Result>>;

/**
 * A function with schemas. Calling it starts the task and waits for its result; use
 * `ctx.start` to exchange messages with the task while it runs.
 */
export type Task<
  Input extends WorkflowSchema = WorkflowSchema,
  Result extends WorkflowSchema = WorkflowSchema,
  Steering extends WorkflowSchema | undefined = WorkflowSchema | undefined,
  Output extends WorkflowSchema | undefined = WorkflowSchema | undefined
> = ((ctx: TaskContext<any, any>, input: SchemaInput<Input>) => Promise<SchemaOutput<Result>>) &
  Readonly<TaskDefinition<Input, Result, Steering, Output>> & {
    /** Internal: validate and run the implementation in a child context. */
    readonly [implementation]: (ctx: TaskContext<any, any>, input: unknown) => Promise<unknown>;
  };

/** Any task, for APIs that accept every task. */
export type AnyTask = Task<any, any, any, any>;
export type TaskInput<T extends AnyTask> = SchemaInput<T['input']>;
export type TaskResult<T extends AnyTask> = SchemaOutput<T['result']>;
/** A message schema of a task definition without the `undefined` of its optional property.
 * It is `never` when the task has no message schema. */
type DefinedSchema<S> = Exclude<S, undefined>;
/** What a parent sends to a task. `unknown` when the task has no `steering` schema. */
export type TaskSteering<T extends AnyTask> = [DefinedSchema<T['steering']>] extends [never]
  ? unknown
  : MessageInput<DefinedSchema<T['steering']>>;
/** Streamed output seen by the parent; `unknown` without an output schema. */
export type TaskOutput<T extends AnyTask> = [DefinedSchema<T['output']>] extends [never]
  ? unknown
  : MessageOutput<DefinedSchema<T['output']>>;
/** The handle of a started task. */
export type TaskInvocation<T extends AnyTask> = Invocation<
  TaskSteering<T>,
  TaskOutput<T>,
  TaskResult<T>
>;

/** Define a task: a named function with input and result schemas, and optional message schemas. */
export function task<
  const Input extends WorkflowSchema,
  const Result extends WorkflowSchema,
  const Steering extends WorkflowSchema | undefined = undefined,
  const Output extends WorkflowSchema | undefined = undefined
>(
  definition: TaskDefinition<Input, Result, Steering, Output>,
  run: TaskImplementation<Input, Result, Steering, Output>
): Task<Input, Result, Steering, Output> {
  const { name } = definition;
  if (typeof name !== 'string' || name.trim() === '') {
    throw new TypeError('Task name must be a non-empty string');
  }
  for (const boundary of ['input', 'result', 'steering', 'output'] as const) {
    const schema = definition[boundary];
    if (schema === undefined && (boundary === 'steering' || boundary === 'output')) continue;
    if (!isWorkflowSchema(schema)) {
      throw new TypeError(`Task ${JSON.stringify(name)} ${boundary} schema is invalid`);
    }
  }

  const execute = async (ctx: TaskContext<any, any>, value: unknown) => {
    const input = await parseBoundary(name, 'input', definition.input, value);
    ctx.signal.throwIfAborted();
    const result = await run(ctx, input as SchemaOutput<Input>);
    return parseBoundary(name, 'result', definition.result, result);
  };

  const defined = (ctx: TaskContext<any, any>, input: unknown) =>
    invoke(ctx, defined as unknown as AnyTask, input, { direct: true }).result;

  Object.defineProperties(defined, {
    name: { value: name },
    input: { value: definition.input, enumerable: true },
    result: { value: definition.result, enumerable: true },
    ...(definition.steering === undefined
      ? {}
      : { steering: { value: definition.steering, enumerable: true } }),
    ...(definition.output === undefined
      ? {}
      : { output: { value: definition.output, enumerable: true } }),
    [implementation]: { value: execute }
  });
  return defined as unknown as Task<Input, Result, Steering, Output>;
}

/** Check whether a value was defined with `task()`. */
export const isTask = (value: unknown): value is AnyTask =>
  typeof value === 'function' &&
  typeof (value as unknown as Record<symbol, unknown>)[implementation] === 'function';
