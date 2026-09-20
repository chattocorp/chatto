import { expect, test } from "vitest";
import { createServeCommand } from "../runtime/cli.ts";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";
import { tmpdir } from "node:os";
import { resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";

test("development server uses the same server options", () => {
  const command = createServeCommand().name("runling-web").exitOverride();
  command.parse(["--port", "4173", "--open"], { from: "user" });
  expect(command.opts()).toEqual({ config: "runling.config.ts", host: "localhost", port: 4173, open: true });
});

test("development server serves the app from outside the repository", async () => {
  const directory = await mkdtemp(resolve(tmpdir(), "runling-dev-"));
  const appRoot = resolve(import.meta.dirname, "../..");
  const portServer = createServer();
  portServer.listen(0, "127.0.0.1");
  await once(portServer, "listening");
  const address = portServer.address();
  if (!address || typeof address === "string") throw new Error("Expected a TCP port");
  await new Promise<void>((resolve, reject) => portServer.close(error => error ? reject(error) : resolve()));
  await writeFile(resolve(directory, "external.config.ts"), "export default { webhooks: {} };\n");
  const server = spawn(process.execPath, [
    "--import", pathToFileURL(createRequire(import.meta.url).resolve("tsx")).href,
    "--conditions=runling-source",
    resolve(appRoot, "src/bin/runling-web.ts"),
    "--config", "external.config.ts",
    "--host", "127.0.0.1", "--port", String(address.port),
  ], { cwd: directory, stdio: ["ignore", "pipe", "pipe"] });
  let output = "";
  server.stdout.on("data", chunk => { output += chunk; });
  server.stderr.on("data", chunk => { output += chunk; });
  const closed = once(server, "close");
  try {
    let response: Response | undefined;
    for (let attempt = 0; attempt < 100; attempt++) {
      if (server.exitCode !== null) throw new Error(output);
      try {
        response = await fetch(`http://127.0.0.1:${address.port}`, { signal: AbortSignal.timeout(30_000) });
        break;
      } catch {
        await delay(100);
      }
    }
    expect(response, output).toBeDefined();
    const html = await response!.text();
    expect(response!.status, html + output).toBe(200);
    expect(html).toContain("<html");
  } finally {
    server.kill("SIGTERM");
    await closed;
    await rm(directory, { recursive: true, force: true });
  }
}, 60_000);
