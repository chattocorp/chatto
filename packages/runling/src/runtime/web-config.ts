import { isSchemaTask, type Task } from './workflow.ts';
import type { WorkflowContext } from './context.ts';
import { toJsonSchema, isWorkflowSchema, type WorkflowSchema } from './schema.ts';

export type WebhookTask<Input, Output> = (ctx: WorkflowContext, input: Input) => Output;

export interface StartedRun {
  id: string;
}

/** Host services for one delivery. Starting a run waits only for registration. */
export interface WebhookContext {
  start<Input, Output>(
    task: WebhookTask<Input, Output>,
    options: { input: Input }
  ): Promise<StartedRun>;
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
export function startWorkflow<Input, Output>(
  rootTask: WebhookTask<Input, Output>
): WebhookRouter<Input> {
  const route: WebhookRouter<Input> = async (ctx, input) => {
    await ctx.start(rootTask, { input });
  };

  route.label = rootTask.name;
  if (isSchemaTask(rootTask)) {
    route.input = rootTask.input;
    route.output = rootTask.output;
  }
  return route;
}

export interface WebConfig<
  Webhooks extends Record<string, WebhookRouter<any>> = Record<string, WebhookRouter<any>>
> {
  webhooks: Webhooks;
  sources?: Record<string, EventSource>;
}

/** Host services for a source generation. State survives reloads of the same source name. */
export interface EventSourceContext {
  signal: AbortSignal;
  /** Process-local adapter state. Removed when the source is removed. Never serialized. */
  state: Map<string, unknown>;
  /** Route one delivery and wait for all initiated registrations. Throws on routing failure. */
  dispatch<Input>(router: WebhookRouter<Input>, input: Input): Promise<StartedRun[]>;
}

/** Run until aborted; settle only after releasing sockets, timers, and pending deliveries. */
export type EventSource = (ctx: EventSourceContext) => Promise<void>;

/** Preserve names and concrete router types in configuration. */
export function defineWebConfig<
  const Webhooks extends Record<string, WebhookRouter<any>> = Record<string, WebhookRouter<any>>
>(config: { webhooks?: Webhooks; sources?: Record<string, EventSource> }): WebConfig<Webhooks> {
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
export function describeTaskSchemas(task: Pick<Task, 'input' | 'output'>) {
  return {
    input: toJsonSchema(task.input, 'input'),
    output: toJsonSchema(task.output, 'output')
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
