import * as runling from "runling";
import type {
  Runling,
  Static,
  TSchema,
  Workflow,
  WorkflowDefinition,
} from "runling";

type TaskFactory = <Input extends TSchema, Output extends TSchema>(
  definition: WorkflowDefinition<Input, Output>,
  run: (
    runtime: Runling,
    input: Static<Input>,
  ) => Static<Output> | Promise<Static<Output>>,
) => Workflow<Input, Output>;

// Published Runling 0.6 calls this workflow(); the linked development checkout
// calls it task(). Keep this adapter until the renamed API has been released.
export const task = (Reflect.get(runling, "task") ??
  Reflect.get(runling, "workflow")) as TaskFactory;
