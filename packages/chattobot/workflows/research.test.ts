import { expect, test, vi } from 'vitest';
import { createWorkflowContext, emptyTokenUsage } from 'runling';
import type { AgentExtensionAPI, AgentOptions } from 'runling/agents';
import { createResearch, researchExtension } from './research.ts';

const cloudflare = { accountId: '0123456789abcdef0123456789abcdef', apiToken: 'cf-token' };

/** Capture the research agent's options and run its registered tools from a fake model turn. */
function fakeResearchAgent(
  turn: (call: (name: string, input: unknown) => Promise<unknown>) => Promise<unknown>
) {
  const created: AgentOptions[] = [];
  const prompts: string[] = [];
  const dispose = vi.fn();
  const createAgent = async (options: AgentOptions) => {
    created.push(options);
    const tools = new Map<string, { execute(id: string, input: never): Promise<unknown> }>();
    for (const extension of options.extensions ?? []) {
      const factory = typeof extension === 'function' ? extension : extension.factory;
      await factory({
        registerTool(tool) {
          tools.set(tool.name, tool as never);
        }
      } as AgentExtensionAPI);
    }
    return {
      async runOutcome(_ctx: unknown, prompt: string) {
        prompts.push(prompt);
        return (await turn((name, input) =>
          tools.get(name)!.execute('call', input as never)
        )) as never;
      },
      dispose
    };
  };
  return { createAgent, created, prompts, dispose };
}

test('the research agent sees only the question and has only web tools', async () => {
  const request = vi.fn<typeof fetch>(async (url) =>
    String(url).includes('tavily')
      ? Response.json({ results: [{ title: 'Guide', url: 'https://found.example/', content: '' }] })
      : Response.json({ success: true, result: '# Page' })
  );
  const fake = fakeResearchAgent(async (call) => {
    await call('webSearch', { query: 'chatto bridges' });
    await call('browsePage', { url: 'https://user.example/page' });
    // The research agent can build URLs on allowed sites itself.
    await call('browsePage', { url: 'https://github.com/chattocorp/chatto/pull/2654' });
    // The supervisor wrote this URL into the question; only user-written URLs are allowed.
    await expect(
      call('browsePage', { url: 'https://attacker.example/?d=summary' })
    ).rejects.toThrow('can open only');
    return {
      outcome: 'completed',
      summary: 'Found bridges',
      details: 'Bridges exist (https://found.example/).',
      usage: emptyTokenUsage()
    };
  });
  const research = createResearch(
    { tavilyApiKey: 'tvly-key', cloudflare },
    {
      createAgent: fake.createAgent,
      request,
      model: 'test/model',
      userText: () => 'What bridges exist? Maybe https://user.example/page helps.'
    }
  );
  const question =
    'Which Chatto bridges exist? See https://user.example/page and https://attacker.example/?d=summary';
  const result = await research(createWorkflowContext(), { question });
  expect(result).toEqual({
    outcome: 'completed',
    answer: 'Bridges exist (https://found.example/).',
    sources: [
      'https://found.example/',
      'https://user.example/page',
      'https://github.com/chattocorp/chatto/pull/2654'
    ]
  });
  expect(fake.prompts).toEqual([question]);
  const [options] = fake.created;
  expect(options).toMatchObject({
    label: 'research',
    model: 'test/model',
    tools: ['webSearch', 'browsePage'],
    resources: {
      extensions: false,
      skills: false,
      promptTemplates: false,
      themes: false,
      contextFiles: false
    }
  });
  expect(options!.output).toBeUndefined();
  expect(fake.dispose).toHaveBeenCalledOnce();
});

test('blocked, provider, and timed-out research return a blocked result', async () => {
  const blocked = fakeResearchAgent(async () => ({
    outcome: 'blocked',
    summary: 'Nothing relevant found',
    usage: emptyTokenUsage()
  }));
  expect(
    await createResearch({ tavilyApiKey: 'k' }, { createAgent: blocked.createAgent })(
      createWorkflowContext(),
      { question: 'q' }
    )
  ).toMatchObject({ outcome: 'blocked', answer: 'Nothing relevant found' });

  const provider = fakeResearchAgent(async () => ({
    outcome: 'failed',
    failureReason: 'provider_error',
    summary: 'raw provider text',
    usage: emptyTokenUsage()
  }));
  expect(
    await createResearch({ tavilyApiKey: 'k' }, { createAgent: provider.createAgent })(
      createWorkflowContext(),
      { question: 'q' }
    )
  ).toMatchObject({ outcome: 'blocked', answer: expect.stringContaining('model provider') });

  const slow = fakeResearchAgent(
    () => new Promise((_resolve, reject) => setTimeout(() => reject(new Error('aborted')), 50))
  );
  expect(
    await createResearch({ tavilyApiKey: 'k' }, { createAgent: slow.createAgent, timeoutMs: 10 })(
      createWorkflowContext(),
      { question: 'q' }
    )
  ).toMatchObject({ outcome: 'blocked', answer: 'The research did not finish in time.' });
  expect(slow.dispose).toHaveBeenCalledOnce();
});

test('parent cancellation fails the research instead of returning a result', async () => {
  const controller = new AbortController();
  const fake = fakeResearchAgent(async () => {
    controller.abort(new Error('Cancelled from Chatto'));
    throw new Error('aborted');
  });
  await expect(
    createResearch({ tavilyApiKey: 'k' }, { createAgent: fake.createAgent })(
      { ...createWorkflowContext(), signal: controller.signal },
      { question: 'q' }
    )
  ).rejects.toThrow();
  expect(fake.dispose).toHaveBeenCalledOnce();
});

test('a completed report at the deadline keeps its answer', async () => {
  const fake = fakeResearchAgent(
    () =>
      new Promise((resolve) =>
        setTimeout(
          () =>
            resolve({
              outcome: 'completed',
              summary: 'Done',
              details: 'Late but complete',
              usage: emptyTokenUsage()
            }),
          30
        )
      )
  );
  expect(
    await createResearch({ tavilyApiKey: 'k' }, { createAgent: fake.createAgent, timeoutMs: 5 })(
      createWorkflowContext(),
      { question: 'q' }
    )
  ).toMatchObject({ outcome: 'completed', answer: 'Late but complete' });
});

test('researchWeb refuses calls beyond the host limit before starting an agent', async () => {
  const fake = fakeResearchAgent(async () => ({
    outcome: 'completed',
    summary: 'Done',
    details: 'Answer',
    usage: emptyTokenUsage()
  }));
  let left = 1;
  const tools = new Map<string, { execute(id: string, input: never): Promise<unknown> }>();
  const extension = researchExtension(
    createWorkflowContext() as never,
    { tavilyApiKey: 'k' },
    { createAgent: fake.createAgent, take: () => left-- > 0 }
  );
  const factory = typeof extension === 'function' ? extension : extension.factory;
  await factory({
    registerTool(tool) {
      tools.set(tool.name, tool as never);
    }
  } as AgentExtensionAPI);
  const research = () => tools.get('researchWeb')!.execute('call', { question: 'q' } as never);
  expect(JSON.stringify(await research())).toContain('Answer');
  await expect(research()).rejects.toThrow('research limit');
  expect(fake.created).toHaveLength(1);
});
