import assert from "node:assert/strict";
import { execFile, spawn } from "node:child_process";
import {
  copyFile,
  mkdtemp,
  mkdir,
  readFile,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { resolve, dirname, delimiter } from "node:path";
import { createServer } from "node:net";
import { promisify } from "node:util";
import { setTimeout as delay } from "node:timers/promises";
import { cleanupConsumer } from "./cleanup-consumer.mjs";

const exec = promisify(execFile);
// Keep npm and its child executables on the runtime used to launch this test.
process.env.PATH = `${dirname(process.execPath)}${delimiter}${process.env.PATH ?? ""}`;
const root = resolve(import.meta.dirname, "..");
const directory = await mkdtemp(resolve(tmpdir(), "runling-consumer-"));
let server;
let serverOutput = "";
try {
  const { stdout } = await exec(
    "npm",
    ["pack", "--json", "--pack-destination", directory],
    { cwd: root },
  );
  const [packed] = JSON.parse(stdout);
  assert.equal(packed.name, "runling");
  const manifest = JSON.parse(
    await readFile(resolve(root, "package.json"), "utf8"),
  );
  assert.equal(packed.version, manifest.version);
  assert(packed.files.some(({ path }) => path === "README.md"));
  assert(packed.files.some(({ path }) => path === "LICENSE"));
  assert(packed.files.some(({ path }) => path === "bin/runling.js"));
  assert(!packed.files.some(({ path }) => path === "bin/factory.js"));
  assert(packed.files.some(({ path }) => path === "dist/web/index.js"));
  assert(packed.files.some(({ path }) => path === "dist/src/runtime/index.d.ts"));
  assert(!packed.files.some(({ path }) => path.endsWith(".test.ts")));
  const project = resolve(directory, "project with spaces");
  await mkdir(project);
  await writeFile(
    resolve(project, "package.json"),
    JSON.stringify({
      name: "runling-consumer-test",
      private: true,
      type: "module",
      scripts: { runling: "runling" },
    }),
  );
  await exec(
    "npm",
    ["install", "--no-audit", "--no-fund", resolve(directory, packed.filename), `zod@${manifest.devDependencies.zod}`],
    { cwd: project, maxBuffer: 8 * 1024 * 1024 },
  );
  await writeFile(resolve(project, "channels.mjs"), `
import assert from "node:assert/strict";
import { createWorkflowContext, createChannel, spawn, task, Type, agent as rootAgent } from "runling";
import { connectAgent, agent, taskTool, runAgentConversation } from "runling/agents";
assert.equal(agent, rootAgent);
const echo = task({ name: "Echo", input: Type.String(), output: Type.String() }, (_ctx, input) => input);
assert.equal(typeof runAgentConversation, "function");
const echoTool = taskTool(createWorkflowContext(), {
  name: "echo", label: "Echo", description: "Echo input", parameters: echo.input,
}, echo);
assert.equal((await echoTool.execute("id", "hello")).content[0].text, "hello");
const connected = connectAgent(createWorkflowContext(), {
  async runOutcome() { return { outcome: "completed", summary: "connected", usage: createWorkflowContext().usage }; },
});
assert.equal((await connected.runOutcome("hello")).summary, "connected");
await connected.dispose();
const worker = task(async ctx => {
  let total = 0;
  for await (const value of ctx.inbox) { total += value; await ctx.emit(total); }
  return total;
});
const child = spawn(createWorkflowContext(), worker);
await child.send(2);
await child.send(3);
child.closeInput();
const updates = [];
for await (const update of child.updates) updates.push(update);
assert.deepEqual(updates, [2, 5]);
assert.equal(await child.result, 5);
const channel = createChannel({ capacity: 1 });
channel.close();
await assert.rejects(channel.send(1), { name: "ChannelClosedError" });
`);
  await exec(process.execPath, ["channels.mjs"], { cwd: project });
  await writeFile(resolve(project, "message.txt"), "consumer cwd");
  await writeFile(
    resolve(project, ".env"),
    "RUNLING_PACKAGE_TEST_ENV=loaded\n",
  );
  await writeFile(resolve(project, "helper.ts"), 'export const suffix = "";\n');
  await writeFile(
    resolve(project, "workflow.ts"),
    `import { task, exec, step, log } from "runling";
import { z } from "zod";
import { suffix } from "./helper.ts";
export default task({ name: "Consumer echo", input: z.object({ topic: z.string(), directory: z.string() }), output: z.string() }, async (ctx, input) => {
  return step("Echo input", async () => {
    await new Promise(resolve => setTimeout(resolve, input.topic === "slow" ? 2000 : 250));
    const cwd = await exec\`node -e \${"process.stdout.write(require('node:fs').readFileSync('message.txt', 'utf8'))"}\`.cwd(input.directory).text();
    log.info(input.topic);
    return input.topic + ": " + cwd + suffix;
  });
});
`,
  );
  await writeFile(
    resolve(project, "cli.ts"),
    `import { task, Type, exec, step, log, createWorkflowContext } from "runling";
import * as git from "runling/git";
import { connectAgent, agent, runAgent, defineAgentExtension } from "runling/agents";
async function checkAgents() {
const connection = connectAgent(createWorkflowContext(), {
  async runOutcome() { return { outcome: "completed", summary: "ok", usage: createWorkflowContext().usage }; },
});
if ((await connection.runOutcome("test")).summary !== "ok") throw new Error("Agent connection failed");
await connection.dispose();
}
if ([agent, runAgent, defineAgentExtension].some(value => typeof value !== "function")) throw new Error("Missing agent exports");
export default task({ name: "CLI echo", input: Type.String(), output: Type.String() }, async (ctx, input) => {
  await checkAgents();
  if (typeof git.getPwd !== "function" || typeof git.workingTreeHash !== "function" || typeof git.WorkingDirectory.create !== "function") throw new Error("Git helpers were not exported");
  if (process.env.RUNLING_PACKAGE_TEST_ENV !== "loaded") throw new Error("Project .env was not loaded");
  if (createWorkflowContext().usage.input !== 0 || "cwd" in ctx) throw new Error("Invalid workflow context");
  ctx.recordUsage({ input: 10, output: 2, cacheRead: 0, cacheWrite: 0, cost: 0.25 });
  return input;
});
`,
  );
  await writeFile(
    resolve(project, "runling.config.ts"),
    `import { defineWebConfig, startWorkflow } from "runling/web";
import echo from "./workflow.ts";
export default defineWebConfig({ webhooks: { echo: startWorkflow(echo) } });
`,
  );
  const cli = await exec(
    "npm",
    ["run", "--silent", "runling", "--", "run", "cli.ts", "explicit input", "--json"],
    { cwd: project },
  );
  assert.equal(JSON.parse(cli.stdout).output, "explicit input");
  assert.deepEqual(JSON.parse(cli.stdout).usage, {
    input: 10, output: 2, cacheRead: 0, cacheWrite: 0, cost: 0.25,
  });
  const structured = await exec(
    "npm",
    ["run", "--silent", "runling", "--", "run", "workflow.ts", "--input", JSON.stringify({ directory: project, topic: "structured" }), "--json"],
    { cwd: project },
  );
  assert.equal(JSON.parse(structured.stdout).output, "structured: consumer cwd");
  // A normal npm project need not declare itself an ESM package.
  await writeFile(
    resolve(project, "package.json"),
    JSON.stringify({
      name: "runling-consumer-test",
      private: true,
      scripts: { runling: "runling" },
      dependencies: { runling: packed.version },
    }),
  );
  const commonjsCli = await exec(
    "npm",
    [
      "run",
      "--silent",
      "runling",
      "--",
      "run",
      "cli.ts",
      "commonjs project",
      "--json",
    ],
    { cwd: project },
  );
  assert.equal(JSON.parse(commonjsCli.stdout).output, "commonjs project");
  await exec(
    process.execPath,
    [
      resolve(root, "node_modules/typescript/bin/tsc"),
      "--noEmit",
      "--strict",
      "--module",
      "nodenext",
      "--target",
      "esnext",
      "--allowImportingTsExtensions",
      "--skipLibCheck",
      "runling.config.ts",
      "workflow.ts",
      "cli.ts",
    ],
    { cwd: project },
  );

  const portServer = createServer();
  await new Promise((resolve) => portServer.listen(0, "127.0.0.1", resolve));
  const port = portServer.address().port;
  await new Promise((resolve) => portServer.close(resolve));
  const origin = `http://127.0.0.1:${port}`;
  server = spawn(
    "npm",
    ["run", "runling", "--", "serve", "--host", "127.0.0.1", "--port", String(port)],
    {
      cwd: project,
      detached: process.platform !== "win32",
      stdio: ["ignore", "pipe", "pipe"],
    },
  );
  server.stdout.on("data", (chunk) => {
    serverOutput += chunk;
  });
  server.stderr.on("data", (chunk) => {
    serverOutput += chunk;
  });
  let response;
  for (let attempt = 0; attempt < 100; attempt++) {
    if (server.exitCode !== null) throw new Error(serverOutput);
    try {
      response = await fetch(origin);
      break;
    } catch {
      await delay(100);
    }
  }
  assert(response, `Server did not start:\n${serverOutput}`);
  const html = await response.text();
  assert.equal(response.status, 200, html + serverOutput);
  assert(html.includes("Consumer echo"));
  const asset = html.match(/(?:href|src)="([^" ]+\.css)"/)?.[1];
  assert(asset, "UI contains a stylesheet");
  assert.equal((await fetch(new URL(asset, origin))).status, 200);
  const schemaResponse = await fetch(`${origin}/api/webhooks/echo`);
  assert.equal(schemaResponse.status, 200);
  const schemas = await schemaResponse.json();
  assert.equal(schemas.input.type, "object");
  assert.equal(schemas.input.properties.topic.type, "string");
  assert.equal(schemas.output.type, "string");
  assert(!JSON.stringify(schemas).includes("~standard"));
  const post = (path, body) =>
    fetch(`${origin}${path}`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(typeof body === "object" ? { ...body, directory: project } : body),
    });
  assert.equal((await post("/api/webhooks/echo", "invalid")).status, 400);
  const webhook = await post("/api/webhooks/echo", { topic: "webhook" });
  assert.equal(webhook.status, 202);
  const waitForRun = async (id) => {
    for (let attempt = 0; attempt < 100; attempt++) {
      const run = await (await fetch(`${origin}/api/runs/${id}`)).json();
      if (run.status !== "running" && run.status !== "waiting") return run;
      await delay(100);
    }
    throw new Error(`Run did not finish: ${id}`);
  };
  const webhookRun = await waitForRun((await webhook.json()).runs[0].id);
  assert.equal(webhookRun.output, "webhook: consumer cwd");
  const workflowPath = resolve(project, "workflow.ts");
  const originalWorkflow = await readFile(workflowPath, "utf8");
  await writeFile(
    workflowPath,
    originalWorkflow.replace("Consumer echo", "Reloaded echo"),
  );
  const waitForPage = async (text) => {
    for (let attempt = 0; attempt < 100; attempt++) {
      if ((await (await fetch(origin)).text()).includes(text)) return;
      await delay(100);
    }
    throw new Error(`Config did not reload: ${text}\n${serverOutput}`);
  };
  await waitForPage("Reloaded echo");
  const active = await post("/api/runs/start/echo", { topic: "slow" });
  const activeId = (await active.json()).runs[0].id;
  await writeFile(
    resolve(project, "helper.ts"),
    'export const suffix = " updated";\n',
  );
  let updated = false;
  for (let attempt = 0; attempt < 30; attempt++) {
    const output = await (
      await post("/api/webhooks/echo", { topic: "new" })
    ).json();
    if ((await waitForRun(output.runs[0].id)).output === "new: consumer cwd updated") {
      updated = true;
      break;
    }
    await delay(100);
  }
  assert(updated, "Transitive imports reload for new runs");
  let oldRun;
  for (let attempt = 0; attempt < 50; attempt++) {
    oldRun = await (await fetch(`${origin}/api/runs/${activeId}`)).json();
    if (oldRun.status !== "running") break;
    await delay(100);
  }
  assert.equal(
    oldRun.output,
    "slow: consumer cwd",
    "Active runs keep their original modules",
  );
  const configPath = resolve(project, "runling.config.ts");
  const configState = async () => {
    const response = await fetch(`${origin}/api/config/events`, {
      signal: AbortSignal.timeout(5000),
    });
    const reader = response.body.getReader();
    let text = "";
    try {
      while (!text.includes("\n\n")) {
        const { value, done } = await reader.read();
        if (done)
          throw new Error("Config event stream closed before its snapshot");
        text += new TextDecoder().decode(value);
      }
      return JSON.parse(
        text
          .split("\n")
          .find((line) => line.startsWith("data: "))
          .slice(6),
      );
    } finally {
      await reader.cancel();
    }
  };
  const originalConfig = await readFile(configPath, "utf8");
  await writeFile(configPath, "export default { broken: true };\n");
  for (
    let attempt = 0;
    attempt < 100 && !serverOutput.includes("Config reload failed");
    attempt++
  )
    await delay(100);
  assert(
    serverOutput.includes("Config reload failed"),
    "Invalid reload is reported",
  );
  assert((await configState()).error, "Browser clients receive reload errors");
  assert.equal(
    (await post("/api/webhooks/echo", { topic: "retained" })).status,
    202,
  );
  await writeFile(configPath, originalConfig.replace("echo: startWorkflow", "renamed: startWorkflow"));
  await waitForPage("/api/webhooks/renamed");
  assert.equal(
    (await configState()).error,
    null,
    "Recovery clears the browser error",
  );
  assert.equal((await fetch(`${origin}/api/webhooks/echo`)).status, 404);
  await writeFile(configPath, originalConfig);
  await waitForPage("/api/webhooks/echo");
  const started = await post("/api/runs/start/echo", { topic: "console" });
  assert.equal(started.status, 202);
  const { runs: [{ id }] } = await started.json();
  const streamAbort = new AbortController();
  const stream = await fetch(`${origin}/api/runs/${id}/events`, {
    signal: streamAbort.signal,
  });
  const reader = stream.body.getReader();
  let streamed = "";
  try {
    await Promise.race([
      (async () => {
        while (!streamed.includes('"type":"finished"')) {
          const { value, done } = await reader.read();
          if (done) break;
          streamed += new TextDecoder().decode(value);
        }
      })(),
      delay(5000).then(() => {
        throw new Error("Timed out waiting for live events: " + streamed);
      }),
    ]);
  } finally {
    streamAbort.abort();
  }
  const detail = await (await fetch(`${origin}/api/runs/${id}`)).json();
  assert.equal(detail.status, "completed");
  assert(
    streamed.includes("command.started"),
    "Workflow and web UI share the event runtime",
  );
  const history = await readFile(
    resolve(project, ".runling/runs", `${id}.jsonl`),
    "utf8",
  );
  assert(history.includes("console: consumer cwd"));
  const serverLog = (await readFile(resolve(project, ".runling/logs/server.jsonl"), "utf8"))
    .trim().split("\n").map((line) => JSON.parse(line));
  assert(serverLog.some((record) => record.event === "server.listening"));
  assert(serverLog.some((record) => record.event === "http.response" && record.status === 200));
  assert(serverLog.some((record) => record.event === "run.finished" && record.runId === id));
  assert(serverLog.some((record) => record.event === "config.reload_failed"));

  if (process.env.RUNLING_RELEASE_DIR) {
    const destination = resolve(process.env.RUNLING_RELEASE_DIR);
    await mkdir(destination, { recursive: true });
    await copyFile(
      resolve(directory, packed.filename),
      resolve(destination, packed.filename),
    );
  }
  console.log(
    "Packed npm consumer passed: CLI, UI/assets, TS config, webhook, live events, and history.",
  );
} catch (error) {
  console.error(serverOutput);
  throw error;
} finally {
  await cleanupConsumer(directory, server);
}
