/** Exercise the exported CLI task with real configuration parsing and a fake model. */
import { generateKeyPairSync } from 'node:crypto';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import { createAgentSession, emptyTokenUsage, isTask, runWorkflow } from 'runling';
import { usePrivateTempDirectory } from 'runling';
import workflow from './github.ts';

vi.mock('runling', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling')>()),
  createAgentSession: vi.fn()
}));

const directories: string[] = [];
afterEach(async () => {
  vi.unstubAllEnvs();
  vi.clearAllMocks();
  for (const directory of directories.splice(0))
    await rm(directory, { recursive: true, force: true });
});

test('the default task accepts CLI text and uses App settings without a Chatto identity', async () => {
  const directory = await mkdtemp(join(await usePrivateTempDirectory(), 'github-cli-test-'));
  directories.push(directory);
  const keyFile = join(directory, 'app.pem');
  const { privateKey } = generateKeyPairSync('rsa', { modulusLength: 2048 });
  await writeFile(keyFile, privateKey.export({ type: 'pkcs8', format: 'pem' }), { mode: 0o600 });
  vi.stubEnv('CHATTO_AGENT_MODEL', 'test/model');
  vi.stubEnv('CHATTO_AGENT_THINKING', 'low');
  vi.stubEnv('CHATTO_GITHUB_APP_CLIENT_ID', 'Iv23liExample1');
  vi.stubEnv('CHATTO_GITHUB_APP_PRIVATE_KEY_FILE', keyFile);
  vi.stubEnv('CHATTO_GITHUB_REPOSITORY', 'org/repo');
  for (const name of ['CHATTO_URL', 'CHATTO_API_KEY', 'CHATTO_MAINTAINER_USER_IDS'])
    vi.stubEnv(name, '');
  const runOutcome = vi.fn(async () => ({
    outcome: 'completed' as const,
    summary: 'moo',
    usage: emptyTokenUsage()
  }));
  const disposed = vi.fn(async () => {});
  vi.mocked(createAgentSession).mockResolvedValue({
    id: 'worker',
    runOutcome,
    run: async () => {
      throw new Error('unused');
    },
    steer: async () => false,
    fork: async () => {
      throw new Error('unused');
    },
    dispose: async () => {},
    [Symbol.asyncDispose]: disposed
  });
  expect(isTask(workflow)).toBe(true);
  expect(await runWorkflow(workflow, { input: 'moo' })).toMatchObject({ ok: true, output: 'moo' });
  expect(createAgentSession).toHaveBeenCalledWith(
    expect.objectContaining({ model: 'test/model', thinkingLevel: 'low', tools: ['gh'] })
  );
  expect(runOutcome).toHaveBeenCalledWith(
    expect.anything(),
    JSON.stringify({ task: 'moo', evidence: '' }),
    expect.objectContaining({ signal: expect.any(AbortSignal), onText: expect.any(Function) })
  );
  expect(disposed).toHaveBeenCalledOnce();
});

test('missing model settings fail before starting an agent', async () => {
  vi.stubEnv('CHATTO_AGENT_MODEL', '');
  expect(await runWorkflow(workflow, { input: 'moo' })).toMatchObject({
    ok: false,
    error: 'Set CHATTO_AGENT_MODEL'
  });
  expect(createAgentSession).not.toHaveBeenCalled();
});

test('missing GitHub settings fail before starting an agent', async () => {
  vi.stubEnv('CHATTO_AGENT_MODEL', 'test/model');
  vi.stubEnv('CHATTO_GITHUB_APP_CLIENT_ID', '');
  vi.stubEnv('CHATTO_GITHUB_APP_PRIVATE_KEY_FILE', '');
  expect(await runWorkflow(workflow, { input: 'moo' })).toMatchObject({
    ok: false,
    error: expect.stringContaining('CHATTO_GITHUB_APP_CLIENT_ID')
  });
  expect(createAgentSession).not.toHaveBeenCalled();
});
