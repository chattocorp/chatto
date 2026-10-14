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
import { createInvestigator, describeEvent } from './investigate.ts';

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
        agentOptions.onEvent?.({
          type: 'tool_execution_start',
          toolCallId: 't',
          toolName: 'read',
          args: { path: 'src/a.ts' }
        } as never);
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
    tools: ['read', 'grep', 'find', 'ls', 'gitLog', 'gitBlame', 'gitShow'],
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

test('reports its tool calls and reasoning steps as updates', async () => {
  const { directory, investigate } = await setup(['See src/a.ts:1.']);
  const investigation = createTaskContext().start(investigate, { directory, question: 'Q?' });
  const updates: unknown[] = [];
  for await (const update of investigation.updates) updates.push(update);
  await investigation.result;
  expect(updates).toEqual(['Reading src/a.ts']);
});

test('describes tool calls and reasoning, but not nested calls', () => {
  const tool = (toolName: string, args: object, parentToolCallId?: string) =>
    describeEvent({
      type: 'tool_execution_start',
      toolCallId: 't',
      toolName,
      args,
      ...(parentToolCallId ? { parentToolCallId } : {})
    } as never);
  expect(tool('grep', { pattern: 'author', path: 'apps' })).toBe('Searching for "author" in apps');
  expect(tool('gitBlame', { path: 'a.go', start: 1, end: 9 })).toBe(
    'Reading the Git blame of a.go:1-9'
  );
  expect(tool('gitShow', { revision: 'abc' })).toBe('Reading commit abc');
  expect(tool('read', { path: 'x' }, 'parent')).toBeUndefined();
  expect(
    describeEvent({
      type: 'message_end',
      message: {
        role: 'assistant',
        content: [{ type: 'thinking', thinking: '**Checking the renderer**\n\nDetails' }]
      }
    } as never)
  ).toBe('Thinking: Checking the renderer');
});

test('starts a new investigation with an ID that the caller chose', async () => {
  const { directory, investigate } = await setup(['First.', 'Second.']);
  const id = '11111111-2222-3333-4444-555555555555';
  expect((await investigate(createTaskContext(), { directory, question: 'Q?', id })).session).toBe(
    id
  );
  await expect(investigate(createTaskContext(), { directory, question: 'Q?', id })).rejects.toThrow(
    `Investigation ${id} exists already`
  );
  await expect(
    investigate(createTaskContext(), { directory, question: 'Q?', id, session: id })
  ).rejects.toThrow('not both');
});
