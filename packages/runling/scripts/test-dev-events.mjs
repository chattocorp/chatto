import assert from 'node:assert/strict';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createServer } from 'vite';

const directory = await mkdtemp(join(tmpdir(), 'runling-dev-events-'));
const configPath = join(directory, 'runling.config.ts');
await writeFile(
  configPath,
  `
  import { task, Type } from "runling";
  import { defineWebConfig } from "runling/web";
  // Return the first message that the host sends to the run.
  const probe = task({
    name: "Event probe", input: Type.String(), result: Type.String(), steering: Type.String(),
  }, async (ctx) => {
    for await (const message of ctx.steering) return message;
    throw new Error("No message");
  });
  export default defineWebConfig({ webhooks: {
    probe: async (ctx, value) => {
      if (value === "skip") return;
      const run = await ctx.start(probe, { input: value });
      await run.steer(value);
    },
  } });
`
);
process.env.RUNLING_WEB_CONFIG = configPath;
// Match pnpm dev: the config loader and web host must use the same native runtime.
const server = await createServer({
  server: { middlewareMode: true }
});
let loader;
try {
  const configModule = await server.ssrLoadModule('/src/lib/server/web-config.ts');
  const { RunStore } = await server.ssrLoadModule('/src/lib/server/run-store.ts');
  loader = configModule.getConfigReloader();
  const config = await loader.load();
  const store = new RunStore(join(directory, 'runs'));
  await store.init();
  let started;
  await config.webhooks.probe(
    {
      start: async (task, { input }) => {
        started = await store.start('probe', task, input, 'web');
        return started;
      }
    },
    'Answer'
  );
  const { id, completion } = started;
  await completion;
  const run = await store.get(id);
  assert.equal(run.status, 'completed');
  assert.equal(run.output, 'Answer');
  // The task can ask for the message before or after it arrives, so a wait is optional.
  const types = run.events
    .map((event) => event.type)
    .filter((type) => type !== 'log' && !type.startsWith('wait.'));
  assert.deepEqual(types, [
    'step.started',
    'task.linked',
    'message.sent',
    'message.read',
    'step.finished'
  ]);
  // Exercise the actual asynchronous HTTP entry point: routing precedes run creation.
  const endpoint = await server.ssrLoadModule('/src/routes/api/runs/start/[name]/+server.ts');
  const { getRunStore } = await server.ssrLoadModule('/src/lib/server/run-store.ts');
  const hostStore = await getRunStore();
  const send = (input) =>
    endpoint.POST({
      params: { name: 'probe' },
      request: new Request('http://localhost/api/runs/start/probe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input)
      })
    });
  const ignored = await send('skip');
  assert.equal(ignored.status, 202);
  assert.deepEqual(await ignored.json(), { runs: [] });
  assert.equal(hostStore.list().length, 0);
  const accepted = await send('Answer');
  assert.equal(accepted.status, 202);
  const acceptedId = (await accepted.json()).runs[0].id;
  for (
    let attempts = 0;
    attempts < 100 && (await hostStore.get(acceptedId)).status === 'running';
    attempts++
  ) {
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  assert.equal((await hostStore.get(acceptedId)).status, 'completed');
  await send('skip');
  assert.equal(hostStore.list().length, 1);
  const restored = new RunStore(join(directory, 'runs'));
  await restored.init();
  assert.deepEqual((await restored.get(id)).events, run.events);
} finally {
  await loader?.close();
  await server.close();
  await rm(directory, { recursive: true, force: true });
}
