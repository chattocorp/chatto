import { mkdtemp, mkdir, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test, vi } from 'vitest';
import {
  createTaskContext,
  emptyTokenUsage,
  type AgentOptions,
  type AgentResult,
  type RunlingAgent
} from 'runling';
import { createInvestigator } from './investigate.ts';

async function setup(answers: (string | AgentResult)[]) {
  const directory = await mkdtemp(join(tmpdir(), 'repo-'));
  await mkdir(join(directory, 'src'));
  await writeFile(join(directory, 'src/a.ts'), 'one\ntwo\nthree\n');
  const sessions = join(await mkdtemp(join(tmpdir(), 'sessions-')), 'investigations');
  const prompts: string[] = [];
  const options: AgentOptions[] = [];
  const createAgent = vi.fn(async (agentOptions: AgentOptions) => {
    options.push(agentOptions);
    // A real agent writes its session file; the fake creates it.
    await writeFile(agentOptions.sessionFile!, '');
    return {
      async runOutcome(_ctx: unknown, prompt: string) {
        prompts.push(prompt);
        const answer = answers.shift()!;
        return typeof answer === 'string'
          ? { outcome: 'completed', summary: answer, usage: emptyTokenUsage() }
          : answer;
      },
      async [Symbol.asyncDispose]() {}
    } as unknown as RunlingAgent;
  });
  const investigate = createInvestigator({ model: 'test/model', sessions }, createAgent);
  return { directory, investigate, prompts, options };
}

test('answers with checked references in a read-only agent in the directory', async () => {
  const { directory, investigate, prompts, options } = await setup([
    'It is in `src/a.ts:2-3`.\n\nChecked: src/a.ts\nNot checked: tests'
  ]);
  const result = await investigate(createTaskContext(), { directory, question: 'Where?' });
  expect(result).toMatchObject({
    answer: expect.stringContaining('src/a.ts:2-3'),
    citations: [{ path: 'src/a.ts', start: 2, end: 3 }]
  });
  expect(result.session).toMatch(/^[0-9a-f-]{36}$/);
  expect(prompts).toEqual(['Where?']);
  expect(options[0]).toMatchObject({
    cwd: directory,
    output: 'text',
    tools: ['read', 'grep', 'find', 'ls', 'git'],
    sessionFile: expect.stringContaining(`${result.session}.jsonl`)
  });
});

test('asks once to correct invalid references and keeps only valid ones', async () => {
  const { directory, investigate, prompts } = await setup([
    'See src/a.ts:9 and src/b.ts:1.',
    'See src/a.ts:1 and src/b.ts:1.'
  ]);
  const result = await investigate(createTaskContext(), { directory, question: 'Where?' });
  expect(prompts[1]).toContain('src/a.ts has 3 lines; lines 9-9 do not exist');
  expect(prompts[1]).toContain('src/b.ts does not exist');
  expect(result.answer).toBe('See src/a.ts:1 and src/b.ts:1.');
  expect(result.citations).toEqual([{ path: 'src/a.ts', start: 1, end: 1 }]);
});

test('resumes an earlier investigation with its session file', async () => {
  const { directory, investigate, options } = await setup(['First.', 'Follow-up.']);
  const first = await investigate(createTaskContext(), { directory, question: 'One?' });
  const second = await investigate(createTaskContext(), {
    directory,
    question: 'Two?',
    session: first.session
  });
  expect(second.session).toBe(first.session);
  expect(options[1]!.sessionFile).toBe(options[0]!.sessionFile);
  await expect(
    investigate(createTaskContext(), {
      directory,
      question: 'Three?',
      session: '00000000-0000-0000-0000-000000000000'
    })
  ).rejects.toThrow('There is no investigation 00000000-0000-0000-0000-000000000000');
});

test('fails when the agent cannot answer or the directory is missing', async () => {
  const { directory, investigate } = await setup([
    { outcome: 'failed', summary: 'Provider error', usage: emptyTokenUsage() }
  ]);
  await expect(investigate(createTaskContext(), { directory, question: 'Q?' })).rejects.toThrow(
    'Provider error'
  );
  await expect(
    investigate(createTaskContext(), { directory: join(directory, 'nope'), question: 'Q?' })
  ).rejects.toThrow('The directory to investigate does not exist');
});
