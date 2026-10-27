/** Verify the workspace programmatic host, shared event runtime, recovery, and cleanup over HTTP. */
import assert from 'node:assert/strict';
import { mkdir, mkdtemp, rm, readFile, writeFile, access } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { serve } from 'runling/server';
import { defineWebConfig } from 'runling/web';
import { log, step, task, Type } from 'runling';
import { setTimeout as delay } from 'node:timers/promises';
import { createServer } from 'node:http';

const scratch = resolve('../../.context');
await mkdir(scratch, { recursive: true });
const directory = await mkdtemp(join(scratch, 'runling-server-'));
const originalDirectory = process.env.RUNLING_DATA_DIRECTORY;
const originalSignals = process.listenerCount('SIGTERM');
let id;
let cleanup = 0;
let priorEvents = [];
try {
  for (let generation = 0; generation < 2; generation++) {
    const ready = Promise.withResolvers();
    let processed = 0;
    const item = task(
      { name: 'Work item', input: Type.Number(), result: Type.Null() },
      async () => {
        await step('Application step', async () => {
          log.warn('Application warning', { items: 1 });
        });
        return null;
      }
    );
    const root = task(
      { name: 'Worker', input: Type.String(), result: Type.Null(), steering: Type.Number() },
      async (ctx) => {
        if (id) assert.equal(ctx.run.id, id);
        id = ctx.run.id;
        for await (const input of ctx.steering) {
          const child = ctx.start(item, input);
          try {
            await child.result;
          } finally {
            await child[Symbol.asyncDispose]();
          }
          if (++processed === 2) ready.resolve();
        }
        return null;
      }
    );
    const config = defineWebConfig({
      workflows: { worker: root },
      sources: {
        example: async (source) => {
          try {
            log.info('Source ready', { generation });
            let run = source
              .runs()
              .find((run) => run.workflow === 'worker' && run.input === 'project');
            if (!run)
              await source.dispatch(async (ctx) => {
                run = await ctx.start(root, { input: 'project' });
              }, null);
            for (let input = 0; input < 2; input++) await run.steer(input);
            await new Promise((resolve) =>
              source.signal.addEventListener('abort', resolve, { once: true })
            );
          } catch (error) {
            ready.reject(error);
            throw error;
          } finally {
            cleanup++;
          }
        }
      }
    });
    const server = await serve({ config, dataDirectory: directory, port: 0, quietLog: true });
    try {
      await Promise.race([
        ready.promise,
        new Promise((_, reject) => {
          AbortSignal.timeout(10000).addEventListener(
            'abort',
            () => reject(new Error('Source startup timed out')),
            { once: true }
          );
        })
      ]);
      assert.equal(process.listenerCount('SIGTERM'), originalSignals + 1);
      await assert.rejects(serve({ config, port: 0 }), /already owns/);
      const page = await fetch(server.url);
      assert.equal(page.status, 200);
      let run;
      const expectedItems = 2 * (generation + 1);
      const deadline = Date.now() + 5000;
      do {
        const response = await fetch(`${server.url}/api/runs/${id}`);
        assert.equal(response.status, 200);
        run = await response.json();
        // Journal appends are asynchronous. A step's start can be visible before
        // its application log; wait for the records asserted below.
        if (
          run.events.filter(
            (event) =>
              event.type === 'log' &&
              event.message === 'Application warning' &&
              event.fields?.items === 1
          ).length === expectedItems
        )
          break;
        await delay(25);
      } while (Date.now() < deadline);
      assert.equal(run.status, 'running');
      assert.equal(
        run.events.filter(
          (event) =>
            event.type === 'log' &&
            event.message === 'Application warning' &&
            event.fields?.items === 1
        ).length,
        expectedItems
      );
      const diagnostics = (await readFile(join(directory, 'logs/server.jsonl'), 'utf8'))
        .trim()
        .split('\n')
        .map((line) => JSON.parse(line));
      assert.equal(
        diagnostics.filter(
          (record) =>
            record.event === 'source.log' &&
            record.source === 'example' &&
            record.message === 'Source ready' &&
            record.fields?.generation === generation
        ).length,
        1
      );
      assert.equal(
        diagnostics.some((record) => record.message === 'Application warning'),
        false
      );
      assert.equal(
        run.events.filter((event) => event.type === 'step.started' && event.label === 'Work item')
          .length,
        expectedItems
      );
      assert.equal(
        run.events.filter(
          (event) => event.type === 'step.started' && event.label === 'Application step'
        ).length,
        expectedItems
      );
      assert.deepEqual(run.events.slice(0, priorEvents.length), priorEvents);
      priorEvents = run.events;
    } finally {
      await Promise.all([server.close(), server.close()]);
    }
    assert.equal(cleanup, generation + 1);
    assert.equal(process.listenerCount('SIGTERM'), originalSignals);
    assert.equal(process.env.RUNLING_DATA_DIRECTORY, originalDirectory);
  }
  assert.match(await readFile(join(directory, 'runs', 'state.json'), 'utf8'), /project/);
  // A bind failure must release ownership, environment, and source resources.
  const occupied = createServer();
  await new Promise((resolve) => occupied.listen(0, '127.0.0.1', resolve));
  try {
    await assert.rejects(
      serve({
        config: defineWebConfig({}),
        dataDirectory: directory,
        port: occupied.address().port
      }),
      { code: 'EADDRINUSE' }
    );
    assert.equal(process.listenerCount('SIGTERM'), originalSignals);
    assert.equal(process.env.RUNLING_DATA_DIRECTORY, originalDirectory);
  } finally {
    await new Promise((resolve) => occupied.close(resolve));
  }
  const final = await serve({
    config: defineWebConfig({}),
    dataDirectory: directory,
    port: 0,
    signals: false
  });
  assert.equal(process.listenerCount('SIGTERM'), originalSignals);
  await final[Symbol.asyncDispose]();
  // A failed final snapshot must still release the singleton store and sources.
  const invalidReady = Promise.withResolvers();
  const invalid = task(
    { name: 'Invalid state', input: Type.Null(), result: Type.Null() },
    async (ctx) => {
      ctx.state.invalid = 1n;
      invalidReady.resolve();
      for await (const _ of ctx.steering);
      return null;
    }
  );
  const broken = await serve({
    config: defineWebConfig({
      sources: {
        fixture: async (source) => {
          await source.dispatch(async (route) => {
            await route.start(invalid, { input: null });
          }, null);
          if (!source.signal.aborted)
            await new Promise((resolve) =>
              source.signal.addEventListener('abort', resolve, { once: true })
            );
        }
      }
    }),
    dataDirectory: join(directory, 'invalid-state'),
    port: 0,
    signals: false
  });
  await invalidReady.promise;
  await assert.rejects(broken.close(), /JSON|BigInt/);
  assert.equal(process.env.RUNLING_DATA_DIRECTORY, originalDirectory);
  // File-based project setup selects storage before any log or run store opens.
  const projectData = join(directory, 'project-data');
  const configPath = join(directory, 'runling.config.ts');
  await writeFile(
    configPath,
    `process.env.RUNLING_DATA_DIRECTORY = ${JSON.stringify(projectData)};\nexport default { webhooks: {} };\n`
  );
  const project = await serve({ config: configPath, port: 0, signals: false });
  await project.close();
  await access(join(projectData, 'runs', 'state.json'));
  await access(join(projectData, 'logs', 'server.jsonl'));
  assert.equal(process.env.RUNLING_DATA_DIRECTORY, originalDirectory);
  console.info('Programmatic server: HTTP, child events, recovery, and cleanup passed.');
} finally {
  await rm(directory, { recursive: true, force: true });
}
