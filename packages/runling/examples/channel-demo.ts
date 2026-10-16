import { log, task, Type, type Static } from 'runling';

const Command = Type.Union([
  Type.Object({ type: Type.Literal('add'), amount: Type.Number() }),
  Type.Object({ type: Type.Literal('label'), label: Type.String() })
]);
const Total = Type.Object({ label: Type.String(), total: Type.Number() });

/** A child that changes its total for each command from its parent and reports every change. */
const accumulate = task(
  { name: 'Accumulate', input: Type.Number(), output: Total, receives: Command, emits: Total },
  async (ctx, initial) => {
    let total = initial;
    let label = 'Counting';

    // Closing the input ends the loop after all queued commands are processed.
    for await (const command of ctx.inbox) {
      if (command.type === 'label') label = command.label;
      else total += command.amount;
      await ctx.emit({ label, total });
    }

    return { label, total };
  }
);

/** A parent reacts to child updates and sends new instructions while it runs. */
export default task(
  {
    name: 'Task channel demo',
    input: Type.Object({ initial: Type.Optional(Type.Number({ default: 0 })) }),
    output: Type.Object({
      label: Type.String(),
      total: Type.Number(),
      updates: Type.Array(Total)
    })
  },
  async (ctx, { initial = 0 }) => {
    await using child = ctx.start(accumulate, initial);
    const updates: Static<typeof Total>[] = [];

    await child.send({ type: 'add', amount: 1 });
    for await (const update of child.updates) {
      updates.push(update);
      log.info(`${update.label}: ${update.total}`);

      if (updates.length === 1) {
        // The child is still running. Change its instructions from the parent.
        await child.send({ type: 'label', label: 'Updated by parent' });
        await child.send({ type: 'add', amount: 10 });

        // No more commands are needed; the child can now finish.
        child.closeInput();
      }
    }

    return { ...(await child.result), updates };
  }
);
