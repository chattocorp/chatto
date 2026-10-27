import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { test } from 'vitest';

test('Vite dev workflows record task and message events in the web host', async () => {
  await promisify(execFile)(process.execPath, ['--import', 'tsx', 'scripts/test-dev-events.mjs'], {
    timeout: 20_000
  });
}, 25_000);
