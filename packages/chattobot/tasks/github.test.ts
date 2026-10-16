import { generateKeyPairSync } from 'node:crypto';
import { mkdtemp, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import {
  createTaskContext,
  emptyTokenUsage,
  type AgentExtensionAPI,
  type AgentOptions,
  type RunlingAgent
} from 'runling';
import { usePrivateTempDirectory } from '../private-temp.ts';
import { createGitHubWorker } from './github.ts';
import type { githubTool } from '../github/tools.ts';
import type { GhRunner } from '../github/gh.ts';

const directories: string[] = [];
afterEach(async () => {
  for (const directory of directories.splice(0))
    await rm(directory, { recursive: true, force: true });
});

const request = 'Update issue 12 with these reproduction steps.';

async function setup(outcome: 'completed' | 'failed' = 'completed', command?: GhRunner) {
  const sessions = await mkdtemp(join(await usePrivateTempDirectory(), 'github-test-'));
  directories.push(sessions);
  let options!: AgentOptions;
  let tool!: ReturnType<typeof githubTool>;
  const prompts: string[] = [];
  const tokens = vi.fn(async () => 'token');
  const run = vi.fn<GhRunner>(
    command ?? (async () => ({ ok: true, output: 'https://github.com/org/repo/issues/12' }))
  );
  const disposed = vi.fn();
  const createAgent = vi.fn(async (input: AgentOptions): Promise<RunlingAgent> => {
    options = input;
    for (const extension of input.extensions ?? []) {
      const factory = typeof extension === 'function' ? extension : extension.factory;
      await factory({
        registerTool: (registered: typeof tool) => {
          tool = registered;
        }
      } as unknown as AgentExtensionAPI);
    }
    return {
      id: 'github-worker',
      run: async () => {
        throw new Error('unused');
      },
      steer: async () => false,
      fork: async () => {
        throw new Error('unused');
      },
      dispose: disposed,
      async runOutcome(ctx, prompt) {
        prompts.push(prompt);
        await tool.execute(
          '1',
          { args: ['issue', 'edit', '12', '--body=New steps'] },
          ctx.signal,
          undefined,
          {} as never
        );
        return {
          outcome,
          summary: outcome === 'completed' ? 'Updated #12.' : 'Provider failed after the edit.',
          usage: emptyTokenUsage()
        };
      },
      async [Symbol.asyncDispose]() {
        disposed();
      }
    };
  });
  const worker = createGitHubWorker(
    {
      model: 'test/model',
      sessions,
      settings: {
        clientId: 'Iv23liExample1',
        repository: 'org/repo',
        privateKey: generateKeyPairSync('rsa', { modulusLength: 2048 }).privateKey
      },
      tokens,
      run
    },
    createAgent
  );
  return { worker, createAgent, tokens, run, prompts, disposed, options: () => options, sessions };
}

test('the worker uses an isolated gh agent and returns its result to its parent', async () => {
  const h = await setup();
  expect(
    await h.worker(createTaskContext(), {
      request,
      proposedWork: 'Append reproduction steps to issue 12',
      evidence: 'Steps from the investigation',
      approvalProposal: { messageId: 'proposal', role: 'bot', body: 'Append these steps?' }
    })
  ).toBe('Updated #12.');
  expect(h.options()).toMatchObject({
    tools: ['gh'],
    resources: { contextFiles: false, extensions: false }
  });
  expect(h.options().sessionFile).toMatch(new RegExp(`${h.sessions}/[0-9a-f-]+\\.jsonl$`));
  expect(JSON.parse(h.prompts[0]!)).toEqual({
    request,
    proposedWork: 'Append reproduction steps to issue 12',
    evidence: 'Steps from the investigation',
    approvalProposal: { messageId: 'proposal', role: 'bot', body: 'Append these steps?' }
  });
  expect(h.run).toHaveBeenCalledWith(
    ['issue', 'edit', '12', '--body=New steps'],
    expect.objectContaining({ repository: 'org/repo' })
  );
  expect(h.tokens).toHaveBeenCalledWith(
    { issues: 'write', metadata: 'read' },
    expect.any(AbortSignal)
  );
  expect(h.disposed).toHaveBeenCalledOnce();
});

test('rejects an empty request before creating an agent or obtaining credentials', async () => {
  const h = await setup();
  await expect(
    h.worker(createTaskContext(), {
      request: '',
      proposedWork: 'Edit issue 12',
      evidence: 'Approved'
    })
  ).rejects.toThrow();
  expect(h.createAgent).not.toHaveBeenCalled();
  expect(h.tokens).not.toHaveBeenCalled();
  expect(h.run).not.toHaveBeenCalled();
});

test('reports agent failure without retrying a write and disposes the worker', async () => {
  const h = await setup('failed');
  await expect(
    h.worker(createTaskContext(), { request, proposedWork: '', evidence: '' })
  ).rejects.toThrow('Provider failed after the edit');
  expect(h.run).toHaveBeenCalledOnce();
  expect(h.disposed).toHaveBeenCalledOnce();
});

test('parent cancellation reaches the CLI command and disposes the worker', async () => {
  const entered = Promise.withResolvers<AbortSignal>();
  const h = await setup('completed', async (_args, { signal }) => {
    entered.resolve(signal);
    return new Promise((_resolve, reject) => {
      signal.addEventListener('abort', () => reject(signal.reason), { once: true });
    });
  });
  const controller = new AbortController();
  const result = h.worker(
    { ...createTaskContext(), signal: controller.signal },
    { request, proposedWork: '', evidence: '' }
  );
  const failed = expect(result).rejects.toThrow('Cancelled');
  const signal = await entered.promise;
  controller.abort(new Error('Cancelled'));
  await failed;
  expect(signal.aborted).toBe(true);
  await vi.waitFor(() => expect(h.disposed).toHaveBeenCalledOnce());
});
