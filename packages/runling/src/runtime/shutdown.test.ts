import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, readFile, readdir, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from 'vitest';

test('repeated shutdown signals flush an idle conversation with a completed child', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'runling-shutdown-'));
  try {
    await promisify(execFile)(
      process.execPath,
      [
        '--import',
        'tsx',
        '--input-type=module',
        '--eval',
        `
      import { RunStore } from ${JSON.stringify(new URL('../lib/server/run-store.ts', import.meta.url).href)};
      import { installShutdown } from ${JSON.stringify(new URL('./shutdown.ts', import.meta.url).href)};
      import { task, Type } from "runling";
      import { setTimeout as delay } from "node:timers/promises";
      const store = new RunStore(process.argv[1]);
      await store.init();
      // Stands in for the HTTP server, which keeps the process alive until shutdown.
      const server = setInterval(() => {}, 1000);
      installShutdown(async () => {
        clearInterval(server);
        setTimeout(() => process.kill(process.pid, "SIGINT"), 5);
        setTimeout(() => process.kill(process.pid, "SIGTERM"), 10);
        await store.close();
      });
      const child = task({ name: "Child", input: Type.Null(), result: Type.String() }, async () => "evidence");
      await store.start("conversation", task({ name: "Conversation", input: Type.Null(), result: Type.Null() }, async ctx => {
        await child(ctx, null);
        try {
          // The conversation is idle: it waits for the next message from the host.
          setTimeout(() => process.kill(process.pid, "SIGINT"), 1);
          for await (const _ of ctx.steering) {}
        } finally { await delay(50, undefined, { ref: false }); }
        return null;
      }), null, "source");
    `,
        directory
      ],
      { timeout: 10_000 }
    );
    const [file] = (await readdir(directory)).filter((file) => file.endsWith('.jsonl'));
    const records = (await readFile(join(directory, file!), 'utf8'))
      .trim()
      .split('\n')
      .map((line) => JSON.parse(line));
    expect(records.at(-1)).toMatchObject({ type: 'finished', status: 'interrupted' });
    expect(records.filter((record) => record.type === 'finished')).toHaveLength(1);
    expect(
      records.some(
        (record) => record.event?.type === 'wait.finished' && record.event.status === 'closed'
      )
    ).toBe(true);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
