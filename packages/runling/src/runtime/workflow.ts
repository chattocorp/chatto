import type { WorkflowContext } from "./context.ts";
import type { TSchema } from "typebox";
import type { StandardSchemaV1 } from "@standard-schema/spec";
import {
  isWorkflowSchema,
  isStandardSchema,
  validateSchema,
  type SchemaIssue,
  type SchemaInput,
  type SchemaOutput,
  type WorkflowSchema,
} from "./schema.ts";
import { step } from "./step.ts";

export interface TaskDefinition<InputSchema extends WorkflowSchema, OutputSchema extends WorkflowSchema> {
  name: string;
  input: InputSchema;
  output: OutputSchema;
}

export type TaskFunction = (ctx: WorkflowContext, ...args: any[]) => any;
export type Task<
  InputSchema extends WorkflowSchema = WorkflowSchema,
  OutputSchema extends WorkflowSchema = WorkflowSchema,
  Run extends TaskFunction = (ctx: WorkflowContext, input: SchemaOutput<InputSchema>) => SchemaInput<OutputSchema> | Promise<SchemaInput<OutputSchema>>,
> = ((ctx: Parameters<Run>[0], input: SchemaInput<InputSchema>) =>
  InputSchema extends StandardSchemaV1 ? Promise<SchemaOutput<OutputSchema>>
    : OutputSchema extends StandardSchemaV1 ? Promise<SchemaOutput<OutputSchema>> : ReturnType<Run>
) & Readonly<TaskDefinition<InputSchema, OutputSchema>>;

const taskMarker = Symbol.for("runling.task");

const validationMessage = (
  name: string,
  boundary: "input" | "output",
  issues: SchemaIssue[],
): string => {
  const details = issues.slice(0, 3).map(({ path, message }) =>
    `${path}: ${message}`,
  ).join("; ");
  return `Task ${JSON.stringify(name)} ${boundary} is invalid${details === "" ? "" : `: ${details}`}`;
};

/** Track a function with an explicit context, preserving its return behavior. */
export function task<Run extends (ctx: WorkflowContext, ...args: never[]) => unknown>(
  run: Run,
): Parameters<Run> extends [unknown, ...unknown[]] ? Run : (
  this: ThisParameterType<Run>,
  ctx: WorkflowContext,
  ...args: Parameters<Run> extends [] ? []
    : Parameters<Run> extends [unknown?, ...infer Args] ? Args : []
) => ReturnType<Run>;
/** Track a function with TypeBox input and output, preserving synchronous results. */
export function task<
  const InputSchema extends TSchema & { "~standard"?: never },
  const OutputSchema extends TSchema & { "~standard"?: never },
  const Run extends (ctx: WorkflowContext, input: SchemaOutput<InputSchema>) => unknown,
>(
  definition: TaskDefinition<InputSchema, OutputSchema>,
  run: Run,
): Task<InputSchema, OutputSchema, Run>;
/** Track a function with Standard Schema validation and parsed input and output. */
export function task<
  const InputSchema extends WorkflowSchema,
  const OutputSchema extends WorkflowSchema,
  const Run extends (ctx: WorkflowContext, input: SchemaOutput<InputSchema>) => SchemaInput<OutputSchema> | Promise<SchemaInput<OutputSchema>>,
>(
  definition: TaskDefinition<InputSchema, OutputSchema>,
  run: Run,
): Task<InputSchema, OutputSchema, Run>;
export function task(
  definitionOrRun: TaskDefinition<WorkflowSchema, WorkflowSchema> | TaskFunction,
  implementation?: TaskFunction,
): TaskFunction {
  const definition = typeof definitionOrRun === "function" ? undefined : definitionOrRun;
  const run = typeof definitionOrRun === "function" ? definitionOrRun : implementation!;
  const name = definition?.name ?? (run.name || "Task");
  if (definition) {
    for (const boundary of ["input", "output"] as const) {
      if (!isWorkflowSchema(definition[boundary])) {
        throw new TypeError(`Task ${JSON.stringify(name)} ${boundary} schema is invalid`);
      }
    }
  }
  const parse = (boundary: "input" | "output", value: unknown) => {
    const check = (result: Awaited<ReturnType<typeof validateSchema>>) => {
      if (result.issues) throw new TypeError(validationMessage(name, boundary, result.issues));
      return result.value;
    };
    const result = validateSchema(definition![boundary], value);
    return result instanceof Promise ? result.then(check) : check(result);
  };
  const standard = definition && (isStandardSchema(definition.input) || isStandardSchema(definition.output));
  const runStandard = async (receiver: unknown, args: [WorkflowContext, ...unknown[]]) => {
    args[0].signal.throwIfAborted();
    const input = await parse("input", args[1]);
    args[0].signal.throwIfAborted();
    return step(name, async () => {
      const output = await Reflect.apply(run, receiver, [args[0], input, ...args.slice(2)]);
      return parse("output", output);
    });
  };
  const defined = function (this: unknown, ...args: [WorkflowContext, ...unknown[]]) {
    if (standard) return runStandard(this, args);
    args[0].signal.throwIfAborted();
    if (definition) args[1] = parse("input", args[1]);
    return step(name, () => {
      const output = Reflect.apply(run, this, args);
      if (!definition) return output;
      if (output != null && typeof output.then === "function") {
        return Promise.resolve(output).then(value => parse("output", value));
      }
      return parse("output", output);
    });
  };
  Object.defineProperties(defined, {
    name: { value: name },
    [taskMarker]: { value: true },
    ...(definition ? {
      input: { value: definition.input, enumerable: true },
      output: { value: definition.output, enumerable: true },
    } : {}),
  });
  return defined;
}

export const isSchemaTask = (value: unknown): value is Task =>
  typeof value === "function" &&
  typeof value.name === "string" &&
  isWorkflowSchema((value as Partial<Task>).input) &&
  isWorkflowSchema((value as Partial<Task>).output);

export const isTask = (value: unknown): value is TaskFunction =>
  typeof value === "function" &&
  ((value as unknown as Record<symbol, unknown>)[taskMarker] === true || isSchemaTask(value));
