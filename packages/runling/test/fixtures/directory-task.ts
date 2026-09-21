import { task, Type, exec } from "runling";

export default task(
  {
    name: "Explicit directory",
    input: Type.Object({ directory: Type.String(), prompt: Type.String() }),
    output: Type.Object({ directory: Type.String(), prompt: Type.String() }),
  },
  async (ctx, { directory, prompt }) => ({
    directory: await exec`${process.execPath} -e ${"process.stdout.write(process.cwd())"}`.cwd(directory).text(),
    prompt,
  }),
);
