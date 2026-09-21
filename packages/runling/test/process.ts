import { spawn } from "node:child_process";
import { Readable } from "node:stream";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";
const loader = pathToFileURL(
  createRequire(import.meta.url).resolve("tsx"),
).href;

/** Run a test subprocess with captured output and a TypeScript loader. */
export function spawnProcess(
  argv: string[],
  options: { cwd?: string; stdout?: string; stderr?: string } = {},
) {
  const [command, ...args] = argv;
  const child = spawn(
    command!,
    command === process.execPath ? ["--import", loader, ...args] : args,
    {
      cwd: options.cwd,
      stdio: ["ignore", "pipe", "pipe"],
    },
  );
  return {
    stdout: Readable.toWeb(child.stdout) as ReadableStream<Uint8Array>,
    stderr: Readable.toWeb(child.stderr) as ReadableStream<Uint8Array>,
    exited: new Promise<number>((resolve, reject) => {
      child.on("error", reject);
      child.on("close", (code) => resolve(code ?? 1));
    }),
  };
}
