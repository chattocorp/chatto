/** Check isolated descriptions, failures, cancellation, and session cleanup without a provider. */
import { expect, test, vi } from 'vitest';
import { createDescription } from './description.ts';
import { createTaskContext } from './context.ts';
import { emptyTokenUsage } from './usage.ts';
import type { RunlingAgent, AgentOptions } from './agent.ts';

vi.mock('./private-temp.ts', () => ({ usePrivateTempDirectory: async () => '/private' }));

function setup() {
  const run = vi.fn<RunlingAgent['run']>().mockResolvedValue({
    outcome: 'completed',
    summary: '"  Storage options  "',
    usage: emptyTokenUsage()
  });
  const dispose = vi.fn();
  const create = vi.fn(
    async (_options: AgentOptions) =>
      ({ run, [Symbol.asyncDispose]: dispose }) as unknown as RunlingAgent
  );
  const describe = createDescription(
    { name: 'Describe', model: 'test/small', instructions: 'One short label.' },
    create
  );
  return { run, dispose, create, describe };
}

test('describes untrusted content in a fresh tool-free session and allows intentional empty output', async () => {
  const h = setup();
  expect(await h.describe(createTaskContext(), 'Compare storage')).toBe('Storage options');
  expect(h.create).toHaveBeenCalledWith(
    expect.objectContaining({
      model: 'test/small',
      thinkingLevel: 'off',
      tools: [],
      logContent: false,
      resources: {
        extensions: false,
        skills: false,
        promptTemplates: false,
        themes: false,
        contextFiles: false
      }
    })
  );
  expect(h.run.mock.calls[0]![1]).toBe(JSON.stringify({ content: 'Compare storage' }));
  h.run.mockResolvedValueOnce({ outcome: 'completed', summary: '""', usage: emptyTokenUsage() });
  expect(await h.describe(createTaskContext(), 'hello')).toBe('');
  expect(h.create).toHaveBeenCalledTimes(2);
  expect(h.dispose).toHaveBeenCalledTimes(2);
});

test.each(['not JSON', 'null', '{}'])(
  'rejects malformed descriptions without retries: %s',
  async (summary) => {
    const h = setup();
    h.run.mockResolvedValueOnce({ outcome: 'completed', summary, usage: emptyTokenUsage() });
    await expect(h.describe(createTaskContext(), 'content')).rejects.toThrow(
      'Invalid description response'
    );
    expect(h.run).toHaveBeenCalledOnce();
    expect(h.dispose).toHaveBeenCalledOnce();
  }
);

test('propagates model failures and rejects late results after cancellation', async () => {
  const h = setup();
  h.run.mockRejectedValueOnce(new Error('Provider failed'));
  await expect(h.describe(createTaskContext(), 'content')).rejects.toThrow('Provider failed');
  const controller = new AbortController();
  h.run.mockImplementationOnce(async () => {
    controller.abort(new Error('Cancelled'));
    return { outcome: 'completed', summary: '"Late"', usage: emptyTokenUsage() };
  });
  await expect(
    h.describe({ ...createTaskContext(), signal: controller.signal }, 'content')
  ).rejects.toThrow('Cancelled');
  await vi.waitFor(() => expect(h.dispose).toHaveBeenCalledTimes(2));
});
