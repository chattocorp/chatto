/** Recorded runs own session paths; application labels cannot escape their storage. */
import { mkdtemp, rm, stat } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { expect, test } from 'vitest';
import { agentSessionFile } from './agent-sessions.ts';

test('session keys are stable, isolated by run, and private', async () => {
  const directory = await mkdtemp(resolve('../../.context/runling-sessions-'));
  try {
    const run = { id: 'one', filesDirectory: resolve(directory, 'one') };
    const file = await agentSessionFile({ run }, '../../conversation');
    expect(file).toBe(await agentSessionFile({ run }, '../../conversation'));
    expect(dirname(file!)).toBe(resolve(run.filesDirectory, 'agents'));
    expect(await agentSessionFile({ run }, 'worker')).not.toBe(file);
    expect(
      await agentSessionFile(
        { run: { id: 'two', filesDirectory: resolve(directory, 'two') } },
        '../../conversation'
      )
    ).not.toBe(file);
    if (process.platform !== 'win32') expect((await stat(dirname(file!))).mode & 0o777).toBe(0o700);
    expect(await agentSessionFile({}, 'conversation')).toBeUndefined();
    expect(await agentSessionFile({})).toBeUndefined();
    const fresh = await agentSessionFile({ run });
    expect(dirname(fresh!)).toBe(resolve(run.filesDirectory, 'agents'));
    expect(await agentSessionFile({ run })).not.toBe(fresh);
    await expect(agentSessionFile({ run }, '')).rejects.toThrow('must not be empty');
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
