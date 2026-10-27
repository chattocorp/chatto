import { isTask, type AnyTask, type Task, type TaskInput, type TaskSteering } from './task.ts';
import { toJsonSchema, isWorkflowSchema, type WorkflowSchema } from './schema.ts';

/** The ID of a run that a delivery started. */
export interface StartedRun {
  id: string;
}

/** A run that a router started. The router can keep it and send it more messages later, for
 * example the next message of a chat conversation. */
export interface RunHandle<Steering = unknown> extends StartedRun {
  /** Validate a message and queue it for the run's top-level task. Rejects when the run has
   * finished or the message is invalid. */
  steer(message: Steering): Promise<void>;
  /** Tell the top-level task that no more messages come. */
  closeSteering(): void;
  /** Cancel the run. Its tasks receive the abort signal and finish their cleanup. */
  cancel(): void;
  /** Resolves when the run has finished, whatever its result. */
  readonly finished: Promise<void>;
}

/** Host services for one delivery. Starting a run waits only for registration. */
export interface WebhookContext {
  start<T extends AnyTask>(
    task: T,
    options: {
      input: TaskInput<T>;
      /** Reactivate a retained, finished run owned by this route and registered task.
       * Rejects if an execution is still active. No prior input or effects are replayed. */
      runId?: string;
    }
  ): Promise<RunHandle<TaskSteering<T>>>;
}

/** A route may start any number of workflows or handle the delivery itself. */
export type WebhookRouter<Input = unknown> = ((ctx: WebhookContext, input: Input) => unknown) & {
  label?: string;
  /** Optional boundary validation and schema discovery. The router receives raw input. */
  input?: WorkflowSchema;
  /** Descriptive output schema for single-workflow routers, not the HTTP response. */
  output?: WorkflowSchema;
};

/** Always register one run with this task as its root. */
export function startWorkflow<T extends AnyTask>(rootTask: T): WebhookRouter<TaskInput<T>> {
  const route: WebhookRouter<TaskInput<T>> = async (ctx, input) => {
    await ctx.start(rootTask, { input });
  };

  route.label = rootTask.name;
  route.input = rootTask.input;
  route.output = rootTask.result;
  return route;
}

export interface WebConfig<
  Webhooks extends Record<string, WebhookRouter<any>> = Record<string, WebhookRouter<any>>
> {
  webhooks: Webhooks;
  sources?: Record<string, EventSource>;
  /** Stable keys for roots that can recover from saved state. Register the same task object
   * that routers start. Renaming a key leaves its saved workflows interrupted. */
  workflows?: Record<string, AnyTask>;
}

/** Host services for a source generation. State survives reloads of the same source name. */
export interface EventSourceContext {
  signal: AbortSignal;
  /** Process-local adapter state. Removed when the source is removed. Never serialized. */
  state: Map<string, unknown>;
  /** Active runs owned by this source, including recovered roots. Opt into retained finished
   * runs to route a new execution by ID. Input is from the latest execution. Finished handles
   * cannot receive messages. Queued messages and adapter state are not restored. */
  runs(options?: { includeFinished?: boolean }): SourceRun[];
  /** Route one delivery and wait for all initiated registrations. Throws on routing failure. */
  dispatch<Input>(router: WebhookRouter<Input>, input: Input): Promise<StartedRun[]>;
}

/** A source-owned run and its latest input. Finished entries have inert execution handles. */
export interface SourceRun extends RunHandle {
  workflow: string;
  input: unknown;
}

/** Run until aborted; settle only after releasing sockets, timers, and pending deliveries. */
export type EventSource = (ctx: EventSourceContext) => Promise<void>;

/** Preserve names and concrete router types in configuration. */
export function defineWebConfig<
  const Webhooks extends Record<string, WebhookRouter<any>> = Record<string, WebhookRouter<any>>
>(config: {
  webhooks?: Webhooks;
  sources?: Record<string, EventSource>;
  workflows?: Record<string, AnyTask>;
}): WebConfig<Webhooks> {
  if (config.workflows !== undefined) {
    if (
      !config.workflows ||
      typeof config.workflows !== 'object' ||
      Array.isArray(config.workflows)
    )
      throw new TypeError('workflows must be a map of root tasks');
    const tasks = new Set<AnyTask>();
    for (const [key, task] of Object.entries(config.workflows)) {
      if (!key.trim() || !isTask(task) || tasks.has(task))
        throw new TypeError('Each workflow must have a unique key and task');
      tasks.add(task);
    }
  }
  if (
    config.sources !== undefined &&
    (typeof config.sources !== 'object' || config.sources === null || Array.isArray(config.sources))
  ) {
    throw new TypeError('sources must be a map of source functions');
  }
  for (const source of Object.values(config.sources ?? {})) {
    if (typeof source !== 'function') throw new TypeError('source must be a function');
  }
  if (
    config.webhooks !== undefined &&
    (typeof config.webhooks !== 'object' ||
      config.webhooks === null ||
      Array.isArray(config.webhooks))
  ) {
    throw new TypeError('webhooks must be a map of routing functions');
  }
  for (const [name, route] of Object.entries(config.webhooks ?? {})) {
    try {
      if (typeof route !== 'function') {
        throw new TypeError('webhook must be a routing function');
      }

      describeRouterSchemas(route);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : String(cause);
      throw new TypeError(`Webhook ${JSON.stringify(name)} is invalid: ${message}`, { cause });
    }
  }
  return { ...config, webhooks: config.webhooks ?? ({} as Webhooks) };
}

export function describeRouterSchemas(route: WebhookRouter<any>) {
  if (route.label !== undefined && typeof route.label !== 'string') {
    throw new TypeError('route label must be a string');
  }
  for (const schema of [route.input, route.output]) {
    if (schema !== undefined && !isWorkflowSchema(schema)) {
      throw new TypeError('route metadata must contain valid schemas');
    }
  }

  return {
    input: route.input === undefined ? {} : toJsonSchema(route.input, 'input'),
    output: route.output === undefined ? {} : toJsonSchema(route.output, 'output')
  };
}

/** Get JSON Schema descriptions for a task. */
export function describeTaskSchemas(task: Pick<Task, 'input' | 'result'>) {
  return {
    input: toJsonSchema(task.input, 'input'),
    output: toJsonSchema(task.result, 'output')
  };
}

export function isWebConfig(value: unknown): value is WebConfig {
  if (
    typeof value !== 'object' ||
    value === null ||
    !('webhooks' in value) ||
    typeof value.webhooks !== 'object' ||
    value.webhooks === null ||
    Array.isArray(value.webhooks)
  ) {
    return false;
  }

  try {
    defineWebConfig(value as WebConfig);
    return true;
  } catch {
    return false;
  }
}
