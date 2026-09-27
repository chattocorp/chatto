import { expect, test, vi } from 'vitest';
import type { ExtensionAPI } from '@earendil-works/pi-coding-agent';
import { createTrustExtension, type TrustPolicy } from './trust.ts';

function install(policy: TrustPolicy, options?: Parameters<typeof createTrustExtension>[1]) {
  const trust = createTrustExtension(policy, options);
  const handlers = new Map<string, (event: unknown) => unknown>();
  trust.extension({
    on: (name: string, handler: (event: unknown) => unknown) => handlers.set(name, handler)
  } as unknown as ExtensionAPI);
  return {
    trust,
    call: async (toolName: string) =>
      await handlers.get('tool_call')!({ type: 'tool_call', toolName, input: {} }),
    result: (toolName: string, isError = false) =>
      handlers.get('tool_result')!({ type: 'tool_result', toolName, isError, content: [] })
  };
}

const policy = { untrusted: ['web_fetch'], blockAfterUntrusted: ['bash', 'write'] };

test('blocks listed tools only after an untrusted result, including an error', async () => {
  const log = vi.fn();
  const agent = install(policy, { log });
  expect(await agent.call('bash')).toBeUndefined();
  agent.result('read');
  expect(agent.trust.untrusted).toBe(false);
  agent.result('web_fetch', true);
  expect(agent.trust.untrusted).toBe(true);
  expect(await agent.call('bash')).toEqual({
    block: true,
    reason: "bash is blocked because this agent's context contains untrusted content."
  });
  expect(await agent.call('read')).toBeUndefined();
  expect(log).toHaveBeenCalledWith('info', 'Blocked bash: untrusted content in context');
});

test('can start untrusted, for example in a fork', async () => {
  const agent = install(policy, { untrusted: true });
  expect(await agent.call('write')).toMatchObject({ block: true });
});

test('runs onBlocked first and reports its failure without the error text', async () => {
  const log = vi.fn();
  const onBlocked = vi.fn(async () => {
    throw new Error('secret host detail');
  });
  const agent = install({ ...policy, onBlocked }, { log, untrusted: true });
  expect(await agent.call('bash')).toMatchObject({ block: true });
  expect(onBlocked).toHaveBeenCalledWith('bash');
  expect(log).toHaveBeenCalledWith('error', 'Blocked-tool callback failed for bash');
  expect(JSON.stringify(log.mock.calls)).not.toContain('secret host detail');
});

test('separate instances keep separate marks', async () => {
  const first = install(policy);
  const second = install(policy);
  first.result('web_fetch');
  expect(await first.call('bash')).toMatchObject({ block: true });
  expect(await second.call('bash')).toBeUndefined();
});
