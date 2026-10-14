import { task, Type } from 'runling';

export default task(
  {
    name: 'Echo',
    input: Type.String(),
    output: Type.Object({ echo: Type.String(), id: Type.String() })
  },
  async (_ctx, input) => ({ echo: input, id: crypto.randomUUID() })
);
