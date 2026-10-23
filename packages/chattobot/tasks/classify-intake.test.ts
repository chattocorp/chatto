import { afterEach, expect, test, vi } from 'vitest';
import { createTaskContext, emptyTokenUsage, type AgentOptions, type RunlingAgent } from 'runling';
import { createIntakeClassifier } from './classify-intake.ts';
import { intakeDecisionSettings } from '../settings.ts';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

test('dedicated participation uses OpenRouter without starting an agent', async () => {
  vi.stubEnv('CHATTO_INTAKE_DECISION_MODEL', 'typesafe/jev-1.13');
  vi.stubEnv('OPENROUTER_API_KEY', 'test-key');
  const request = vi.fn<typeof fetch>(async () =>
    Response.json({
      answers: {
        decision: {
          type: 'choice',
          choice: 'ignore',
          confidence: 0.9,
          probabilities: { engage: 0.1, ignore: 0.9, like: 0, love: 0, smile: 0 }
        }
      }
    })
  );
  vi.stubGlobal('fetch', request);
  const create = vi.fn();
  const classify = createIntakeClassifier('test/agent', intakeDecisionSettings(), create);
  expect(await classify(createTaskContext(), { body: 'Hello', attachmentTypes: [] })).toEqual({
    action: 'ignore',
    probabilities: { engage: 0.1, ignore: 0.9, like: 0, love: 0, smile: 0 }
  });
  expect(create).not.toHaveBeenCalled();
  expect(JSON.parse(String(request.mock.calls[0]![1]!.body)).model).toBe('typesafe/jev-1.13');
});

test('unset decision model preserves agent mode; a configured model requires credentials', () => {
  vi.stubEnv('CHATTO_INTAKE_DECISION_MODEL', ' ');
  vi.stubEnv('OPENROUTER_API_KEY', '');
  expect(intakeDecisionSettings()).toEqual({});
  vi.stubEnv('CHATTO_INTAKE_DECISION_MODEL', 'typesafe/jev-1.13');
  expect(() => intakeDecisionSettings()).toThrow('Set OPENROUTER_API_KEY');
});

test('decision request failures do not start an agent fallback', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response('', { status: 503 }))
  );
  const create = vi.fn();
  const classify = createIntakeClassifier(
    'test/agent',
    { model: 'test/decisions', apiKey: 'test-key' },
    create
  );
  await expect(
    classify(createTaskContext(), { body: 'Hello', attachmentTypes: [] })
  ).rejects.toThrow('HTTP 503');
  expect(create).not.toHaveBeenCalled();
});

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
  return { classifier: createIntakeClassifier('test/model', {}, create), create, run, dispose };
}

test.each(['engage', 'ignore', 'like', 'love', 'smile'])(
  'returns a validated decision (%s) from one tool-free turn',
  async (action) => {
    const h = harness(JSON.stringify({ action, reason: 'Fixture decision' }));
    const input = { body: 'Images do not load', attachmentTypes: ['image/png'] };
    expect(await h.classifier(createTaskContext(), input)).toEqual({
      action,
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
  '{"action":"react","emoji":"anything"}',
  '{"action":"love","messageId":"elsewhere"}',
  '{"action":"engage","authorId":"maintainer"}',
  '```json\n{"engage":true,"reason":"report"}\n```',
  '{"engage":"true","reason":"report"}',
  '{"reason":"report"}',
  '{"engage":true,"reason":"report","authorId":"maintainer"}'
])('rejects invalid model output without retrying: %s', async (output) => {
  const h = harness(output);
  await expect(
    h.classifier(createTaskContext(), { body: 'hello', attachmentTypes: [] })
  ).rejects.toThrow(/Invalid classification decision|result is invalid/);
  expect(h.run).toHaveBeenCalledOnce();
  expect(h.dispose).toHaveBeenCalledOnce();
});
