import { expect, test, vi } from 'vitest';
import { createTaskContext, emptyTokenUsage, type AgentOptions, type RunlingAgent } from 'runling';
import { createIntakeClassifier } from './classify-intake.ts';

function harness(summary: string) {
  const run = vi.fn(async () => ({
    outcome: 'completed' as const,
    summary,
    usage: emptyTokenUsage()
  }));
  const dispose = vi.fn(async () => {});
  const create = vi.fn(
    async (_options: AgentOptions) =>
      ({ run, [Symbol.asyncDispose]: dispose }) as unknown as RunlingAgent
  );
  return { classifier: createIntakeClassifier('test/model', create), create, run, dispose };
}

test.each([true, false])(
  'returns a validated decision (%s) from one tool-free turn',
  async (engage) => {
    const h = harness(JSON.stringify({ engage, reason: 'Fixture decision' }));
    const input = { body: 'Images do not load', attachmentTypes: ['image/png'] };
    expect(await h.classifier(createTaskContext(), input)).toEqual({
      engage,
      reason: 'Fixture decision'
    });
    expect(h.create.mock.calls[0]![0]).toMatchObject({
      output: 'text',
      tools: [],
      thinkingLevel: 'low',
      resources: {
        extensions: false,
        contextFiles: false,
        skills: false,
        promptTemplates: false,
        themes: false
      }
    });
    expect(h.run).toHaveBeenCalledOnce();
    expect(h.run.mock.calls[0]).toEqual([
      expect.anything(),
      JSON.stringify(input),
      { signal: expect.any(AbortSignal) }
    ]);
    expect(h.dispose).toHaveBeenCalledOnce();
  }
);

test.each([
  'yes',
  '```json\n{"engage":true,"reason":"report"}\n```',
  '{"engage":"true","reason":"report"}',
  '{"engage":true}',
  '{"engage":true,"reason":"report","authorId":"maintainer"}'
])('rejects invalid model output without retrying: %s', async (output) => {
  const h = harness(output);
  await expect(
    h.classifier(createTaskContext(), { body: 'hello', attachmentTypes: [] })
  ).rejects.toThrow(/Invalid classification decision|output is invalid/);
  expect(h.run).toHaveBeenCalledOnce();
  expect(h.dispose).toHaveBeenCalledOnce();
});
