import type { Static, TSchema } from "typebox";
import type { WorkflowContext } from "../context.ts";

/** Adapt an explicit task call to an agent tool with a text result. */
export function taskTool<Context extends WorkflowContext<unknown, never>, Schema extends TSchema>(
  ctx: Context,
  definition: { name: string; label: string; description: string; parameters: Schema },
  run: (ctx: Context, input: Static<Schema>) => string | Promise<string>,
) {
  return {
    ...definition,
    async execute(_id: string, input: Static<Schema>, signal?: AbortSignal) {
      const toolCtx = signal
        ? { ...ctx, signal: AbortSignal.any([ctx.signal, signal]) }
        : ctx;

      toolCtx.signal.throwIfAborted();
      const text = await run(toolCtx, input);
      return { content: [{ type: "text" as const, text }], details: {} };
    },
  };
}
