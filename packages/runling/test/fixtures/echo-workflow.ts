import { randomId, task, Type } from "runling";

export default task(
  {
    name: "Echo",
    input: Type.String(),
    output: Type.Object({
      summary: Type.String(),
      outputs: Type.Object({ id: Type.String() }),
    }),
  },
  function echo(ctx, input) {
    const id = randomId();
    return {
      summary: input,
      outputs: { id },
    };
  },
);
