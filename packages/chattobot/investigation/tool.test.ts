import { expect, test } from 'vitest';
import { createTaskContext, Type, task, type AgentExtensionAPI } from 'runling';
import { investigateExtension, type Investigation } from './tool.ts';

type Tool = { execute(id: string, input: unknown, signal?: AbortSignal): Promise<any> };

/** The tool over a fake investigator that records its inputs. */
async function tool(views = 2, repository?: string) {
  const inputs: unknown[] = [];
  const investigate = task(
    {
      name: 'Investigate',
      input: Type.Object({
        directory: Type.String(),
        question: Type.String(),
        session: Type.Optional(Type.String())
      }),
      output: Type.Any()
    },
    async (_ctx, input) => {
      inputs.push(input);
      return {
        session: input.session ?? '11111111-1111-1111-1111-111111111111',
        answer: 'See `src/a.ts:2`.',
        commit: 'abcdef0123456789',
        citations: [{ path: 'src/a.ts', start: 2, end: 2 }]
      };
    }
  ) as unknown as Investigation['investigate'];
  let left = views;
  const extension = investigateExtension(
    createTaskContext(),
    { directory: '/repo', investigate, ...(repository ? { repository } : {}) },
    { take: () => left-- > 0 }
  );
  const tools = new Map<string, Tool>();
  const factory = typeof extension === 'function' ? extension : extension.factory;
  await factory({
    registerTool: (registered: { name: string }) =>
      void tools.set(registered.name, registered as never)
  } as unknown as AgentExtensionAPI);
  return { investigate: tools.get('investigate')!, inputs };
}

test('starts an investigation in the configured directory and links its references', async () => {
  const { investigate, inputs } = await tool(2, 'org/repo');
  const result = await investigate.execute('1', { question: 'Where?' });
  expect(inputs).toEqual([{ directory: '/repo', question: 'Where?' }]);
  expect(result.content[0].text).toBe(
    'Investigation 11111111-1111-1111-1111-111111111111 (code at commit abcdef012):\n\nSee [`src/a.ts:2`](https://github.com/org/repo/blob/abcdef0123456789/src/a.ts#L2).\n\nFor a follow-up question on this topic, call investigate with resume: "11111111-1111-1111-1111-111111111111". This investigator remembers what it read.'
  );
});

test('resumes an investigation and keeps references as text without a repository', async () => {
  const { investigate, inputs } = await tool();
  const result = await investigate.execute('1', {
    question: 'And?',
    resume: '22222222-2222-2222-2222-222222222222'
  });
  expect(inputs[0]).toMatchObject({ session: '22222222-2222-2222-2222-222222222222' });
  expect(result.content[0].text).toContain('See `src/a.ts:2`.');
});

test('refuses investigations over the turn budget', async () => {
  const { investigate, inputs } = await tool(1);
  await investigate.execute('1', { question: 'One' });
  await expect(investigate.execute('2', { question: 'Two' })).rejects.toThrow(
    'limit of investigations'
  );
  expect(inputs).toHaveLength(1);
});
