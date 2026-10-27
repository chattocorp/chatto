import { expect, test, vi } from 'vitest';
import { Type } from 'typebox';
import { z } from 'zod';
import { createClassifier } from './classifier.ts';
import { createTaskContext } from './context.ts';
import type { AgentOptions, RunlingAgent } from './agent.ts';
import { emptyTokenUsage } from './usage.ts';

vi.mock('./private-temp.ts', () => ({ usePrivateTempDirectory: async () => '/private' }));

const options = {
  name: 'Classify',
  label: 'classifier',
  model: 'test/model',
  systemPrompt: 'Classify the input.',
  input: Type.String(),
  result: Type.Object({ accept: Type.Boolean() }, { additionalProperties: false })
};
function session(reply: string) {
  const run = vi.fn<RunlingAgent['run']>(async () => ({
    outcome: 'completed',
    summary: reply,
    usage: emptyTokenUsage()
  }));
  const dispose = vi.fn();
  const create = vi.fn(
    async (_options: AgentOptions) =>
      ({ run, [Symbol.asyncDispose]: dispose }) as unknown as RunlingAgent
  );
  return { run, dispose, create };
}

test('runs exactly one isolated turn and validates the decision', async () => {
  const h = session('{"accept":true}');
  const classify = createClassifier({ ...options, logContent: false }, h.create);
  expect(await classify(createTaskContext(), 'question')).toEqual({ accept: true });
  expect(h.create).toHaveBeenCalledWith(
    expect.objectContaining({
      model: options.model,
      output: 'text',
      thinkingLevel: 'low',
      logContent: false,
      tools: [],
      cwd: '/private',
      resources: {
        extensions: false,
        skills: false,
        promptTemplates: false,
        themes: false,
        contextFiles: false
      }
    })
  );
  expect(h.run).toHaveBeenCalledOnce();
  expect(h.run.mock.calls[0]![1]).toBe('"question"');
  expect(h.dispose).toHaveBeenCalledOnce();
});

test.each(['yes', '{"accept":"yes"}', '{"accept":true,"extra":"instruction"}'])(
  'invalid output is a failure, never an implicit decision: %s',
  async (reply) => {
    const h = session(reply);
    await expect(
      createClassifier(options, h.create)(createTaskContext(), 'question')
    ).rejects.toThrow();
    expect(h.run).toHaveBeenCalledOnce();
    expect(h.dispose).toHaveBeenCalledOnce();
  }
);

test('Standard Schema output transformations run once', async () => {
  const transform = vi.fn((value: string) => value.length);
  const h = session('"answer"');
  const classify = createClassifier(
    { ...options, result: z.string().transform(transform) },
    h.create
  );
  expect(await classify(createTaskContext(), 'question')).toBe(6);
  expect(transform).toHaveBeenCalledOnce();
});

test('model failure and cancellation dispose the session without making a decision', async () => {
  const h = session('{"accept":true}');
  h.run.mockRejectedValueOnce(new Error('Provider failed'));
  const classify = createClassifier(options, h.create);
  await expect(classify(createTaskContext(), 'question')).rejects.toThrow('Provider failed');
  const controller = new AbortController();
  h.run.mockImplementationOnce(async () => {
    controller.abort(new Error('Cancelled'));
    return { outcome: 'completed', summary: '{"accept":true}', usage: emptyTokenUsage() };
  });
  await expect(
    classify({ ...createTaskContext(), signal: controller.signal }, 'question')
  ).rejects.toThrow('Cancelled');
  await vi.waitFor(() => expect(h.dispose).toHaveBeenCalledTimes(2));
});
