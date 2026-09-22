import { log, task, Type, type WorkflowContext } from "runling";

type Command =
  | { type: "add"; amount: number }
  | { type: "label"; label: string };

type Update = { label: string; total: number };

const accumulate = task(async function accumulate(
  ctx: WorkflowContext<Command, Update>,
  initial: number,
) {
  let total = initial;
  let label = "Counting";

  // Closing the inbox ends the loop after all queued commands are processed.
  for await (const command of ctx.inbox) {
    if (command.type === "label") {
      label = command.label;
    } else {
      total += command.amount;
    }

    await ctx.emit({ label, total });
  }

  return { label, total };
});

/** A parent reacts to child updates and sends new instructions while it runs. */
export default task(
  {
    name: "Task channel demo",
    input: Type.Object({ initial: Type.Optional(Type.Number({ default: 0 })) }),
    output: Type.Object({
      label: Type.String(),
      total: Type.Number(),
      updates: Type.Array(
        Type.Object({ label: Type.String(), total: Type.Number() }),
      ),
    }),
  },
  async (ctx, { initial = 0 }) => {
    const run = ctx.spawn((ctx: WorkflowContext<Command, Update>) =>
      accumulate(ctx, initial));
    const updates: Update[] = [];

    try {
      await run.send({ type: "add", amount: 1 });

      for await (const update of run.output) {
        updates.push(update);
        log.info(`${update.label}: ${update.total}`);

        if (updates.length === 1) {
          // The child is still running. Change its instructions from the parent.
          await run.send({ type: "label", label: "Updated by parent" });
          await run.send({ type: "add", amount: 10 });

          // No more commands are needed; the child can now finish.
          run.closeInput();
        }
      }

      return { ...(await run.result), updates };
    } finally {
      // Also clean up if sending or consuming an update fails.
      await run[Symbol.asyncDispose]();
    }
  },
);
