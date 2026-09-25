import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, readFile, writeFile, rm, readdir, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import { createWorkflowContext, emptyTokenUsage } from 'runling';
import type { AgentExtensionAPI, AgentOptions, AgentRunOptions } from 'runling/agents';
import { createAgentTasks } from 'runling/agents';
import {
  createImplementation,
  implementationCommandEnvKeys,
  implementationExtension,
  implementationSettings,
  matchesRepository,
  validationDiagnostic,
  workerStopReason
} from './implement.ts';
import {
  implementationProcess,
  ImplementationCommandError,
  type ImplementationProcess
} from './implementation-process.ts';

const exec = promisify(execFile);
const folders: string[] = [];
afterEach(async () => {
  vi.unstubAllEnvs();
  await Promise.all(
    folders.splice(0).map((folder) => rm(folder, { recursive: true, force: true }))
  );
});
async function fixture() {
  const folder = await mkdtemp(join(tmpdir(), 'chattobot-implement-'));
  folders.push(folder);
  const directory = join(folder, 'repo');
  const remote = join(folder, 'remote.git');
  await exec('git', ['init', '--initial-branch=main', directory]);
  const git = async (...args: string[]) =>
    (await exec('git', args, { cwd: directory })).stdout.trim();
  await git('config', 'user.name', 'Test');
  await git('config', 'user.email', 'test@example.invalid');
  await writeFile(join(directory, 'example.txt'), 'original\n');
  await git('add', '.');
  await git('-c', 'commit.gpgsign=false', 'commit', '-m', 'fixture');
  await exec('git', ['init', '--bare', remote]);
  await git('remote', 'add', 'origin', remote);
  await git('push', 'origin', 'main');
  const settings = {
    directory,
    repository: 'example/chatto',
    artifactsDirectory: join(folder, 'artifacts')
  };
  const calls: { command: string; args: string[] }[] = [];
  let published = false;
  let body = '';
  let branch = '';
  const execute: ImplementationProcess = async (command, args, options) => {
    calls.push({ command, args });
    if (command === 'mise') {
      if (args.includes('install')) return '';
      return implementationProcess('bash', ['-c', check], options);
    }
    if (command === 'git' && args.includes('get-url')) return 'git@github.com:example/chatto.git\n';
    if (command === 'gh') {
      if (args[0] === 'auth') return '';
      if (args[1] === 'checks') return JSON.stringify([{ bucket: 'pass' }]);
      if (args[1] === 'create') {
        branch = args[args.indexOf('--head') + 1]!;
        body = await readFile(args[args.indexOf('--body-file') + 1]!, 'utf8');
        published = true;
        return 'https://github.com/example/chatto/pull/7\n';
      }
      if (!published) throw new Error('No PR');
      return JSON.stringify({
        url: 'https://github.com/example/chatto/pull/7',
        headRefName: branch,
        headRefOid: await git('rev-parse', branch),
        baseRefName: 'main',
        state: 'OPEN'
      });
    }
    return implementationProcess(command, args, options);
  };
  return {
    settings,
    execute,
    git,
    remote,
    calls,
    get body() {
      return body;
    }
  };
}

type Tool = {
  name: string;
  execute: (
    id: string,
    input: never,
    signal?: AbortSignal
  ) => Promise<{ isError?: boolean; content: { text?: string }[] }>;
};
async function workerTools(options: AgentOptions) {
  const tools = new Map<string, Tool>();
  for (const extension of options.extensions ?? []) {
    const factory = typeof extension === 'function' ? extension : extension.factory;
    await factory({
      registerTool(tool: Tool) {
        tools.set(tool.name, tool);
      }
    } as unknown as AgentExtensionAPI);
  }
  return async (name: string, input: object) => tools.get(name)!.execute('call', input as never);
}
const patch =
  'diff --git a/example.txt b/example.txt\n--- a/example.txt\n+++ b/example.txt\n@@ -1 +1 @@\n-original\n+fixed\n';
const check = 'test "$(cat example.txt)" = fixed';
const proposal = {
  title: 'fix(example): correct the value',
  summary: 'Correct the value to fix the reported behavior.',
  notes: ['Browser behavior was not checked.']
};
function worker(
  action: (options: AgentOptions, call: Awaited<ReturnType<typeof workerTools>>) => Promise<void>,
  outcome: 'completed' | 'blocked' = 'completed'
) {
  return async (options: AgentOptions) => ({
    dispose: vi.fn(),
    async runOutcome(_ctx: unknown, _prompt: string, runOptions?: AgentRunOptions) {
      runOptions?.onText?.('Worker commentary for the supervisor');
      await action(options, await workerTools(options));
      return {
        outcome,
        summary: 'Model-authored URL must not be trusted: https://example.invalid/pr',
        usage: emptyTokenUsage()
      };
    }
  });
}

test('implements in an isolated worktree, records final checks, pushes and verifies a ready PR', async () => {
  const f = await fixture();
  await writeFile(join(f.settings.directory, 'example.txt'), 'user dirty change\n');
  const originalBranch = await f.git('branch', '--show-current');
  const updates: unknown[] = [];
  const implement = createImplementation(f.settings, {
    execute: f.execute,
    createAgent: worker(async (options, call) => {
      expect(options.tools).toContain('apply_patch');
      expect(options.tools).not.toContain('write');
      expect(options.tools).not.toContain('bash');
      expect(options.tools).toContain('runCheck');
      expect(options.tools).toContain('runFocusedTests');
      expect(options.tools).toContain('reviewDiff');
      expect(options.tools).toContain('checkpointWork');
      expect(options.instructions?.join('\n')).toContain(
        'Partial progress, task size, and a later human quality review are not by themselves blockers.'
      );
      expect(f.calls.some((call) => call.args.includes('install'))).toBe(true);
      expect(await readFile(join(options.cwd, 'example.txt'), 'utf8')).toBe('original\n');
      await call('apply_patch', { patch });
      await call('preparePullRequest', proposal);
    })
  });
  const result = await implement(
    {
      ...createWorkflowContext(),
      emit: async (value) => {
        updates.push(value);
      }
    },
    { request: 'Fix the value' }
  );
  expect(result).toMatchObject({
    outcome: 'completed',
    prUrl: 'https://github.com/example/chatto/pull/7',
    commit: expect.stringMatching(/^[0-9a-f]{40}$/),
    summary: proposal.summary,
    notes: proposal.notes,
    checks: [
      { command: 'mise x -- pnpm run check', passed: true },
      { command: 'mise x -- pnpm run test', passed: true }
    ]
  });
  expect(f.body).toContain('## Verification');
  expect(f.body).toContain('mise x -- pnpm run test');
  expect(f.calls.find((call) => call.args[1] === 'create')?.args).not.toContain('--draft');
  expect(await f.git('branch', '--show-current')).toBe(originalBranch);
  expect(await readFile(join(f.settings.directory, 'example.txt'), 'utf8')).toBe(
    'user dirty change\n'
  );
  expect(
    (await exec('git', ['--git-dir', f.remote, 'show', `${result.branch}:example.txt`])).stdout
  ).toBe('fixed\n');
  expect(updates).toContainEqual(
    expect.objectContaining({ type: 'finding', text: expect.stringContaining('validation passed') })
  );
  expect(updates).toContainEqual({ type: 'output', text: 'Worker commentary for the supervisor' });
  expect(updates).toContainEqual({
    type: 'state',
    activity: 'Validating · mise x -- pnpm run test',
    value: {
      phase: 'validating',
      currentCheck: 'mise x -- pnpm run test',
      completedChecks: ['mise x -- pnpm run check'],
      pendingChecks: ['mise x -- pnpm run test']
    }
  });
  expect(updates).toContainEqual({
    type: 'state',
    value: {
      phase: 'published',
      prUrl: result.prUrl,
      completedChecks: ['mise x -- pnpm run check', 'mise x -- pnpm run test'],
      pendingChecks: []
    }
  });
  const [folder] = await readdir(f.settings.artifactsDirectory);
  expect(
    JSON.parse(
      await readFile(join(f.settings.artifactsDirectory, folder!, 'metadata.json'), 'utf8')
    )
  ).toMatchObject({ stage: 'published', prUrl: result.prUrl });
});

test('worker can review new changes and run an approved check before host validation', async () => {
  const f = await fixture();
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    createAgent: worker(async (options, call) => {
      await call('apply_patch', { patch });
      const diff = await call('reviewDiff', {});
      expect(diff.content[0]?.text).toContain('+fixed');
      expect(diff.content[0]?.text).toContain('-original');
      expect(
        (await exec('git', ['diff', '--cached', '--name-only'], { cwd: options.cwd })).stdout
      ).toBe('');
      const check = await call('runCheck', { check: 'check' });
      expect(check.content[0]?.text).toContain('Passed: mise x -- pnpm run check');
      await call('preparePullRequest', proposal);
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
  expect(
    f.calls.filter((call) => call.command === 'mise' && call.args.at(-1) === 'check')
  ).toHaveLength(2);
});

test('worker can inspect one changed file and run selected frontend tests', async () => {
  const f = await fixture();
  const spec = 'src/lib/example.test.ts';
  await mkdir(join(f.settings.directory, 'apps/frontend/src/lib'), { recursive: true });
  await writeFile(join(f.settings.directory, 'apps/frontend', spec), 'test fixture\n');
  await f.git('add', '.');
  await f.git('-c', 'commit.gpgsign=false', 'commit', '-m', 'Add test fixture');
  await f.git('push', 'origin', 'main');
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', { patch });
      await call('apply_patch', {
        patch: '--- /dev/null\n+++ b/other.txt\n@@ -0,0 +1 @@\n+other\n'
      });
      const fileDiff = await call('reviewDiff', { path: 'example.txt' });
      expect(fileDiff.content[0]?.text).toContain('+fixed');
      expect(fileDiff.content[0]?.text).not.toContain('other.txt');
      expect((await call('reviewDiff', { path: '../example.txt' })).content[0]?.text).toContain(
        'exact changed path'
      );
      const before = f.calls.length;
      expect(
        (await call('runFocusedTests', { project: 'server', files: ['../example.spec.ts'] }))
          .content[0]?.text
      ).toContain('Select existing frontend spec paths');
      expect(f.calls).toHaveLength(before);
      const focused = await call('runFocusedTests', { project: 'server', files: [spec] });
      expect(focused.content[0]?.text).toContain('Passed: mise x -- pnpm --dir apps/frontend');
      expect((await call('runCheck', { check: 'lint:frontend' })).content[0]?.text).toContain(
        'Passed: mise x -- pnpm run lint:frontend'
      );
      expect((await call('runCheck', { check: 'build:frontend' })).content[0]?.text).toContain(
        'Passed: mise x -- pnpm run build:frontend'
      );
      await call('preparePullRequest', proposal);
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
  expect(result.workerChecks).toEqual([
    {
      command: `mise x -- pnpm --dir apps/frontend exec vitest run --project=server ${spec}`,
      passed: true
    },
    { command: 'mise x -- pnpm run lint:frontend', passed: true },
    { command: 'mise x -- pnpm run build:frontend', passed: true }
  ]);
});

test('an actionable checkpoint continues in the same worker before host validation', async () => {
  const f = await fixture();
  const updates: unknown[] = [];
  let turns = 0;
  const createAgent = vi.fn(async (options: AgentOptions) => ({
    dispose: vi.fn(),
    async runOutcome(_ctx: unknown, prompt: string) {
      const call = await workerTools(options);
      turns++;
      if (turns === 1) {
        await call('apply_patch', { patch });
        await call('checkpointWork', {
          summary: 'First file is complete.',
          nextSteps: ['Add the regression fixture'],
          risks: []
        });
      } else {
        expect(prompt).toContain('Add the regression fixture');
        expect(await readFile(join(options.cwd, 'example.txt'), 'utf8')).toBe('fixed\n');
        await call('apply_patch', {
          patch: '--- /dev/null\n+++ b/regression.txt\n@@ -0,0 +1 @@\n+covered\n'
        });
        await call('preparePullRequest', proposal);
      }
      return { outcome: 'completed' as const, summary: 'Continue', usage: emptyTokenUsage() };
    }
  }));
  const result = await createImplementation(f.settings, { createAgent, execute: f.execute })(
    {
      ...createWorkflowContext(),
      emit: async (value) => {
        updates.push(value);
      }
    },
    { request: 'Fix' }
  );
  expect(result.outcome).toBe('completed');
  expect(turns).toBe(2);
  expect(createAgent).toHaveBeenCalledOnce();
  expect(updates).toContainEqual(
    expect.objectContaining({
      type: 'state',
      value: { phase: 'editing_checkpoint', idleCheckpoints: 0 }
    })
  );
  expect(result.checks.every((check) => check.passed)).toBe(true);
});

test('three checkpoints without source progress stop with a retained handoff', async () => {
  const f = await fixture();
  let turns = 0;
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    createAgent: async (options) => ({
      dispose() {},
      async runOutcome() {
        turns++;
        await (
          await workerTools(options)
        )('checkpointWork', {
          summary: 'Need another work turn.',
          nextSteps: ['Edit example.txt'],
          risks: []
        });
        return { outcome: 'completed' as const, summary: 'Continue', usage: emptyTokenUsage() };
      }
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('blocked');
  expect(result.summary).toContain('without source progress');
  expect(turns).toBe(3);
  expect(result.checks).toEqual([]);
  expect(
    JSON.parse(await readFile(join(result.worktree, '..', 'metadata.json'), 'utf8')).handoff
  ).toMatchObject({ nextSteps: ['Edit example.txt'] });
});

test('worker check failures return bounded repair diagnostics and leave final validation to the host', async () => {
  const f = await fixture();
  let workerCheck = true;
  const result = await createImplementation(f.settings, {
    execute: async (command, args, options) => {
      if (command === 'mise' && args.at(-1) === 'check' && workerCheck) {
        workerCheck = false;
        throw new ImplementationCommandError(
          'Failed',
          'Assertion failed for test@example.invalid at https://example.invalid'
        );
      }
      return f.execute(command, args, options);
    },
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', { patch });
      const check = await call('runCheck', { check: 'check' });
      expect(check.isError).toBeUndefined();
      expect(check.content[0]?.text).toContain('Assertion failed');
      expect(check.content[0]?.text).toContain('also failed on the base commit');
      expect(check.content[0]?.text).not.toContain('test@example.invalid');
      expect(check.content[0]?.text).not.toContain('https://example.invalid');
      await call('preparePullRequest', proposal);
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
  expect(result.workerChecks).toEqual([
    { command: 'mise x -- pnpm run check', passed: false, baseline: 'failed' }
  ]);
  expect(result.checks.every((check) => check.passed)).toBe(true);
});

test('a worker check failure that passes on base is marked for worktree repair', async () => {
  const f = await fixture();
  let workerCheck = true;
  const result = await createImplementation(f.settings, {
    execute: async (command, args, options) => {
      if (command === 'mise' && args.at(-1) === 'check' && workerCheck) {
        workerCheck = false;
        throw new ImplementationCommandError('Check failed', 'Changed test failed');
      }
      if (command === 'mise' && args.at(-1) === 'check' && options.cwd.endsWith('/baseline'))
        return '';
      return f.execute(command, args, options);
    },
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', { patch });
      const check = await call('runCheck', { check: 'check' });
      expect(check.content[0]?.text).toContain('passed on the base commit');
      await call('preparePullRequest', proposal);
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
  expect(result.workerChecks).toEqual([
    { command: 'mise x -- pnpm run check', passed: false, baseline: 'passed' }
  ]);
});

test('worker answer tool emits a correlated reply without treating commentary as an answer', async () => {
  const f = await fixture();
  const updates: unknown[] = [];
  const questionId = '3df53a4d-4240-4086-89b2-c37330b92715';
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    createAgent: worker(async (_options, call) => {
      const answer = await call('answerOwner', {
        questionId,
        answer: 'A diff viewer and a test runner would help.'
      });
      expect(answer.content[0]?.text).toBe('Answer sent to the owner.');
      await call('apply_patch', { patch });
      await call('preparePullRequest', proposal);
    })
  })(
    {
      ...createWorkflowContext(),
      emit: async (value) => {
        updates.push(value);
      }
    },
    { request: 'Fix' }
  );
  expect(result.outcome).toBe('completed');
  expect(updates).toContainEqual({
    type: 'reply',
    replyTo: questionId,
    text: 'A diff viewer and a test runner would help.'
  });
  expect(updates).toContainEqual({ type: 'output', text: 'Worker commentary for the supervisor' });
});

test('owner question tool forwards a marked question to the active implementation', async () => {
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx, { progressIntervalMs: 120_000 });
  const finish = Promise.withResolvers<string>();
  const handle = tasks.start(
    'Chatto implementation',
    async (child) => {
      const message = (await child.inbox[Symbol.asyncIterator]().next()).value!;
      const match = /^\[ChattoBot owner question: ([0-9a-f-]{36})\]\n(.+)$/.exec(message);
      expect(match?.[2]).toBe('What tools would help?');
      await child.emit({
        type: 'reply',
        text: 'A diff viewer and a test runner.',
        replyTo: match?.[1]
      });
      return finish.promise;
    },
    undefined
  );
  const call = await workerTools({
    cwd: '/unused',
    model: 'test/model',
    extensions: [
      implementationExtension(
        ctx,
        { directory: '/unused', repository: 'example/chatto' },
        async () => {},
        tasks
      )
    ]
  });
  const reader = tasks.notifications[Symbol.asyncIterator]();
  try {
    const queued = JSON.parse(
      (await call('askImplementation', { id: handle.id, question: 'What tools would help?' }))
        .content[0]!.text!
    );
    expect(queued).toEqual({ queued: true, questionId: expect.any(String) });
    const notice = JSON.parse((await reader.next()).value!);
    expect(notice.type).toBe('task.reply');
    expect(tasks.get(handle.id).output.at(-1)).toMatchObject({
      kind: 'reply',
      text: 'A diff viewer and a test runner.',
      replyTo: queued.questionId
    });
  } finally {
    finish.resolve('done');
    await tasks.dispose();
  }
});

test.each([
  'protected-file',
  'empty',
  'blocked',
  'missing-proposal',
  'history-changed',
  'branch-changed'
])('does not publish %s', async (mode) => {
  const f = await fixture();
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    createAgent: worker(
      async (options, call) => {
        if (mode !== 'empty') await call('apply_patch', { patch });
        if (mode === 'protected-file')
          await writeFile(join(options.cwd, 'AGENTS.md'), 'changed instructions');
        if (mode !== 'missing-proposal') await call('preparePullRequest', proposal);
        if (mode === 'history-changed')
          await exec('git', ['-c', 'commit.gpgsign=false', 'commit', '-am', 'worker commit'], {
            cwd: options.cwd
          });
        if (mode === 'branch-changed')
          await exec('git', ['checkout', '-b', 'unexpected-branch'], { cwd: options.cwd });
      },
      mode === 'blocked' ? 'blocked' : 'completed'
    )
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('blocked');
  expect(result.prUrl).toBeUndefined();
  expect(f.calls.some((call) => call.args.includes('push') || call.args[1] === 'create')).toBe(
    false
  );
});

test('host validation returns diagnostics to the same worker and checks the repaired final tree', async () => {
  const f = await fixture();
  let turns = 0;
  let validations = 0;
  const createAgent = vi.fn(async (options: AgentOptions) => ({
    dispose: vi.fn(),
    async runOutcome(_ctx: unknown, prompt: string) {
      const call = await workerTools(options);
      if (++turns === 1) await call('apply_patch', { patch });
      else {
        expect(prompt).toContain('Expected regression coverage');
        await call('apply_patch', {
          patch: '--- /dev/null\n+++ b/regression.txt\n@@ -0,0 +1 @@\n+covered\n'
        });
      }
      await call('preparePullRequest', proposal);
      return { outcome: 'completed' as const, summary: 'Ready', usage: emptyTokenUsage() };
    }
  }));
  const result = await createImplementation(f.settings, {
    createAgent,
    execute: async (command, args, options) => {
      if (command === 'mise' && args.at(-1) === 'check' && ++validations === 1)
        throw new ImplementationCommandError('Check failed', 'Expected regression coverage');
      if (command === 'mise' && args.at(-1) === 'check' && options.cwd.endsWith('/baseline'))
        return '';
      return f.execute(command, args, options);
    }
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
  expect(createAgent).toHaveBeenCalledOnce();
  expect(turns).toBe(2);
  expect(result.checks.every((check) => check.passed)).toBe(true);
});

test('a failed worker report stops immediately and preserves the worktree for user direction', async () => {
  const f = await fixture();
  let turns = 0;
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    createAgent: async (options) => ({
      dispose() {},
      async runOutcome() {
        turns++;
        await (
          await workerTools(options)
        )('apply_patch', { patch });
        return { outcome: 'failed', summary: 'Edits are unfinished', usage: emptyTokenUsage() };
      }
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(turns).toBe(1);
  expect(result).toMatchObject({
    outcome: 'blocked',
    summary: expect.stringContaining('Edits are unfinished'),
    checks: [],
    workerChecks: []
  });
  expect(result.notes).toContain('Host final validation did not run. No PR was created.');
  expect(await readFile(join(result.worktree, 'example.txt'), 'utf8')).toBe('fixed\n');
  expect(f.calls.some((call) => call.command === 'mise' && !call.args.includes('install'))).toBe(
    false
  );
  expect(f.calls.some((call) => call.args.includes('push') || call.args[1] === 'create')).toBe(
    false
  );
});

test('a new human request can continue the same unfinished worktree and rerun final checks', async () => {
  const f = await fixture();
  const ownerKey = 'conversation-owner';
  const first = await createImplementation(f.settings, {
    execute: f.execute,
    ownerKey,
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', { patch });
      await call('saveHandoff', {
        summary: 'The value is fixed, but the PR description is not prepared.',
        nextSteps: ['Review the diff and prepare the PR'],
        risks: []
      });
    }, 'blocked')
  })(createWorkflowContext(), { request: 'Fix the value' });
  expect(first.outcome).toBe('blocked');
  const resumed = await createImplementation(f.settings, {
    execute: f.execute,
    ownerKey,
    createAgent: async (options) => ({
      dispose() {},
      async runOutcome(_ctx: unknown, prompt: string) {
        expect(JSON.parse(prompt).handoff).toMatchObject({
          nextSteps: ['Review the diff and prepare the PR']
        });
        expect(await readFile(join(options.cwd, 'example.txt'), 'utf8')).toBe('fixed\n');
        const call = await workerTools(options);
        await call('preparePullRequest', proposal);
        return { outcome: 'completed' as const, summary: 'Ready', usage: emptyTokenUsage() };
      }
    })
  })(createWorkflowContext(), { request: 'Continue', resumeArtifactId: first.artifactId });
  expect(resumed).toMatchObject({
    outcome: 'completed',
    branch: first.branch,
    worktree: first.worktree,
    prUrl: 'https://github.com/example/chatto/pull/7'
  });
  expect(resumed.checks.map((check) => check.passed)).toEqual([true, true]);
  const metadata = JSON.parse(await readFile(join(first.worktree, '..', 'metadata.json'), 'utf8'));
  expect(metadata).toMatchObject({
    stage: 'published',
    ownerKey,
    input: { request: 'Fix the value' }
  });
});

test('an unfinished worktree cannot be resumed by a different conversation', async () => {
  const f = await fixture();
  const first = await createImplementation(f.settings, {
    execute: f.execute,
    ownerKey: 'first-conversation',
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', { patch });
    }, 'blocked')
  })(createWorkflowContext(), { request: 'Fix' });
  expect(first.outcome).toBe('blocked');
  const createAgent = vi.fn(worker(async () => {}));
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    ownerKey: 'other-conversation',
    createAgent
  })(createWorkflowContext(), { request: 'Continue', resumeArtifactId: first.artifactId });
  expect(result).toMatchObject({ outcome: 'blocked', worktree: '' });
  expect(createAgent).not.toHaveBeenCalled();
});

test('continuation requires the exact artifact ID', async () => {
  const f = await fixture();
  const ownerKey = 'conversation-owner';
  const first = await createImplementation(f.settings, {
    execute: f.execute,
    ownerKey,
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', { patch });
    }, 'blocked')
  })(createWorkflowContext(), { request: 'Fix' });
  const createAgent = vi.fn(worker(async () => {}));
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    ownerKey,
    createAgent
  })(createWorkflowContext(), { request: 'Continue', resumeArtifactId: 'implementation-missing' });
  expect(result).toMatchObject({ outcome: 'blocked', worktree: '' });
  expect(result.artifactId).not.toBe(first.artifactId);
  expect(createAgent).not.toHaveBeenCalled();
});

test('repository checks remove bot routing and model credentials from the inherited environment', () => {
  expect(
    implementationCommandEnvKeys({
      CHATTO_ALLOWED_USER_ID: 'id',
      OPENAI_API_KEY: 'secret',
      GH_TOKEN: 'token',
      PATH: '/bin'
    })
  ).toEqual(['CHATTO_ALLOWED_USER_ID', 'OPENAI_API_KEY', 'GH_TOKEN']);
});

test('setup, worker checks, and final validation use the isolated command environment', async () => {
  const f = await fixture();
  vi.stubEnv('CHATTO_ALLOWED_USER_ID', 'private-test-id');
  const checkEnvironments: Array<string[] | undefined> = [];
  const result = await createImplementation(f.settings, {
    execute: async (command, args, options) => {
      if (command === 'mise') checkEnvironments.push(options.unsetEnv);
      return f.execute(command, args, options);
    },
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', { patch });
      await call('runCheck', { check: 'check' });
      await call('preparePullRequest', proposal);
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
  expect(checkEnvironments).toHaveLength(4);
  expect(checkEnvironments.every((keys) => keys?.includes('CHATTO_ALLOWED_USER_ID'))).toBe(true);
});

test('patch errors give the worker Git diagnostics and incorrect hunk counts can be repaired', async () => {
  const f = await fixture();
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    createAgent: worker(async (_options, call) => {
      const bad = await call('apply_patch', { patch: patch.replace('-original', '-not present') });
      expect(bad.isError).toBe(true);
      expect(bad.content[0]?.text).toContain('patch does not apply');
      const fixed = await call('apply_patch', {
        patch: patch.replace('@@ -1 +1 @@', '@@ -1,8 +1,20 @@')
      });
      expect(fixed.isError).toBeUndefined();
      await call('preparePullRequest', proposal);
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
});

test.each([
  { path: 'apps/frontend/example.txt', scripts: ['check:frontend', 'test:frontend'] },
  { path: 'cli/example.go', scripts: ['check', 'test', 'test-cli'] }
])('host selects validation for $path', async ({ path, scripts }) => {
  const f = await fixture();
  const validation: string[] = [];
  const result = await createImplementation(f.settings, {
    execute: async (command, args, options) => {
      if (command === 'mise' && !args.includes('install')) {
        validation.push(args.at(-1)!);
        return '';
      }
      return f.execute(command, args, options);
    },
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', {
        patch: `--- /dev/null\n+++ b/${path}\n@@ -0,0 +1 @@\n+fixture\n`
      });
      await call('preparePullRequest', proposal);
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
  expect(validation).toEqual(scripts);
});

test.each(['failed-check', 'source-changing-check'])(
  'stops without publication for %s',
  async (mode) => {
    const f = await fixture();
    let turns = 0;
    let checks = 0;
    const createAgent = vi.fn(
      worker(async (_options, call) => {
        if (++turns === 1) await call('apply_patch', { patch });
        await call('preparePullRequest', proposal);
      })
    );
    const result = await createImplementation(f.settings, {
      createAgent,
      execute: async (command, args, options) => {
        if (command === 'mise' && args.at(-1) === 'check') {
          if (mode === 'failed-check')
            throw new ImplementationCommandError('Check failed', 'Assertion failed');
          await writeFile(join(options.cwd, 'generated.txt'), String(++checks));
        }
        return f.execute(command, args, options);
      }
    })(createWorkflowContext(), { request: 'Fix' });
    expect(result).toMatchObject({ outcome: 'blocked' });
    if (mode === 'failed-check') expect(result.summary).toContain('also failed on the base commit');
    else expect(result.summary).toContain('three attempts');
    if (mode === 'failed-check')
      expect(result.checks).toEqual([
        { command: 'mise x -- pnpm run check', passed: false, diagnostic: 'Assertion failed' }
      ]);
    expect(turns).toBe(mode === 'failed-check' ? 1 : 3);
    expect(createAgent).toHaveBeenCalledOnce();
    expect(f.calls.some((call) => call.args.includes('push'))).toBe(false);
  }
);

test.each(['failed', 'changed-source'])('setup %s stops before worker creation', async (mode) => {
  const f = await fixture();
  const createAgent = vi.fn();
  const result = await createImplementation(f.settings, {
    createAgent,
    execute: async (command, args, options) => {
      if (command === 'mise' && args.includes('install')) {
        if (mode === 'failed') throw new Error('Private setup output');
        await writeFile(join(options.cwd, 'example.txt'), 'unexpected setup change');
      }
      return f.execute(command, args, options);
    }
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('blocked');
  expect(JSON.stringify(result)).not.toContain('Private setup output');
  expect(createAgent).not.toHaveBeenCalled();
  expect(f.calls.some((call) => call.args.includes('push'))).toBe(false);
});

test('cancellation during host validation disposes the same worker and prevents publication', async () => {
  const f = await fixture();
  const controller = new AbortController();
  const dispose = vi.fn();
  await expect(
    createImplementation(f.settings, {
      createAgent: async (options) => ({
        dispose,
        async runOutcome() {
          const call = await workerTools(options);
          await call('apply_patch', { patch });
          await call('preparePullRequest', proposal);
          return { outcome: 'completed', summary: 'Ready', usage: emptyTokenUsage() };
        }
      }),
      execute: async (command, args, options) => {
        if (command === 'mise' && args.at(-1) === 'check') {
          controller.abort(new Error('Cancelled check'));
          options.signal.throwIfAborted();
        }
        return f.execute(command, args, options);
      }
    })({ ...createWorkflowContext(), signal: controller.signal }, { request: 'Fix' })
  ).rejects.toThrow('Cancelled check');
  expect(dispose).toHaveBeenCalledOnce();
  expect(f.calls.some((call) => call.args.includes('push'))).toBe(false);
});

test('notifications cannot restart a failed implementation; a new human request can', async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx);
  let version: number | undefined = 1;
  const announce = vi.fn(async () => {});
  const extension = implementationExtension(ctx, f.settings, announce, tasks, {
    requestVersion: () => version,
    execute: f.execute,
    createAgent: worker(async () => {}, 'blocked')
  });
  const call = await workerTools({
    cwd: f.settings.directory,
    model: 'test/model',
    extensions: [extension]
  });
  const input = { request: 'Fix', announcement: "I'll implement this." };
  try {
    await call('implementChatto', input);
    const duplicate = await call('implementChatto', input);
    expect(JSON.parse(duplicate.content[0]!.text!).outcome).toBe('blocked');
    await vi.waitFor(() =>
      expect(tasks.list().some((task) => task.status === 'running')).toBe(false)
    );
    expect(JSON.parse((await call('implementChatto', input)).content[0]!.text!).outcome).toBe(
      'blocked'
    );
    version = undefined;
    expect(JSON.parse((await call('implementChatto', input)).content[0]!.text!).outcome).toBe(
      'blocked'
    );
    expect(announce).toHaveBeenCalledOnce();
    version = 2;
    expect(JSON.parse((await call('implementChatto', input)).content[0]!.text!).status).toBe(
      'running'
    );
    await vi.waitFor(() =>
      expect(tasks.list().some((task) => task.status === 'running')).toBe(false)
    );
    expect(tasks.list()).toHaveLength(2);
  } finally {
    await tasks.dispose();
  }
});

test.each(['sent', 'failed'])(
  'a stopped implementation records notice delivery: %s',
  async (mode) => {
    const f = await fixture();
    const ctx = createWorkflowContext();
    const tasks = createAgentTasks(ctx, { notifyActivity: false });
    const onStopped = vi.fn(async (_message: string) => {
      if (mode === 'failed') throw new Error('Post failed');
    });
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const extension = implementationExtension(ctx, f.settings, async () => {}, tasks, {
      execute: f.execute,
      createAgent: worker(async () => {}, 'blocked'),
      onStopped
    });
    const call = await workerTools({
      cwd: f.settings.directory,
      model: 'test/model',
      extensions: [extension]
    });
    try {
      const handle = JSON.parse(
        (await call('implementChatto', { request: 'Fix', announcement: 'Starting' })).content[0]!
          .text!
      );
      await vi.waitFor(() => expect(tasks.get(handle.id).status).toBe('completed'));
      expect(onStopped).toHaveBeenCalledExactlyOnceWith(
        expect.stringContaining('The implementation stopped:')
      );
      expect(onStopped.mock.calls[0]![0]).toContain('worktree was kept for review');
      expect(onStopped.mock.calls[0]![0]).toContain('Please tell me how you want to proceed');
      expect(JSON.parse(tasks.get(handle.id).result!)).toMatchObject({
        outcome: 'blocked',
        noticeDelivered: mode === 'sent'
      });
    } finally {
      warning.mockRestore();
      await tasks.dispose();
    }
  }
);

test('a blocked worker reason and its check count reach the owner without claiming final validation', async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx, { notifyActivity: false });
  const onStopped = vi.fn(async (_message: string) => {});
  const extension = implementationExtension(ctx, f.settings, async () => {}, tasks, {
    execute: f.execute,
    onStopped,
    createAgent: async (options) => ({
      dispose() {},
      async runOutcome() {
        const call = await workerTools(options);
        await call('apply_patch', { patch });
        await call('runCheck', { check: 'check' });
        await call('saveHandoff', {
          summary: 'The requested source work remains incomplete.',
          nextSteps: ['Finish the remaining files'],
          risks: []
        });
        return {
          outcome: 'blocked',
          summary: '18 catalog sections remain; translation review belongs in the PR notes.',
          usage: emptyTokenUsage()
        };
      }
    })
  });
  const call = await workerTools({
    cwd: f.settings.directory,
    model: 'test/model',
    extensions: [extension]
  });
  try {
    const handle = JSON.parse(
      (await call('implementChatto', { request: 'Fix', announcement: 'Starting' })).content[0]!
        .text!
    );
    await vi.waitFor(() => expect(tasks.get(handle.id).status).toBe('completed'));
    expect(onStopped).toHaveBeenCalledOnce();
    const message = onStopped.mock.calls[0]![0];
    expect(message).toContain('18 catalog sections remain');
    expect(message).toContain('worker ran 1 check');
    expect(message).toContain('not final host checks');
    expect(message).toContain('implementation-');
    const result = JSON.parse(tasks.get(handle.id).result!);
    expect(result).toMatchObject({
      outcome: 'blocked',
      checks: [],
      workerChecks: [{ command: 'mise x -- pnpm run check', passed: true }],
      noticeDelivered: true
    });
    expect(
      f.calls.filter((entry) => entry.command === 'mise' && entry.args.at(-1) === 'check')
    ).toHaveLength(1);
  } finally {
    await tasks.dispose();
  }
});

test('host posts the verified PR before a separate CI result and suppresses duplicate notices', async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx, { notifyActivity: false });
  const messages: string[] = [];
  const observeChecks = vi.fn(async () => {
    expect(messages).toHaveLength(1);
    expect(messages[0]).toContain('https://github.com/example/chatto/pull/7');
    return { status: 'failed' as const, passed: 1, failed: 1, pending: 0, skipped: 0 };
  });
  const extension = implementationExtension(ctx, f.settings, async () => {}, tasks, {
    execute: f.execute,
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', { patch });
      await call('preparePullRequest', proposal);
    }),
    observeChecks,
    onPublished: async (message) => {
      messages.push(message);
    },
    onCiResult: async (message) => {
      messages.push(message);
    }
  });
  const call = await workerTools({
    cwd: f.settings.directory,
    model: 'test/model',
    extensions: [extension]
  });
  try {
    const handle = JSON.parse(
      (await call('implementChatto', { request: 'Fix', announcement: 'Starting' })).content[0]!
        .text!
    );
    await vi.waitFor(() => expect(tasks.get(handle.id).status).toBe('completed'));
    expect(messages).toHaveLength(2);
    expect(messages[1]).toContain('CI failed');
    expect(JSON.parse(tasks.get(handle.id).result!)).toMatchObject({
      outcome: 'completed',
      ci: { status: 'failed', failed: 1 },
      noticeDelivered: true
    });
  } finally {
    await tasks.dispose();
  }
});

test('a failed PR notice stays available in the terminal task result', async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx, { notifyActivity: false });
  const onCiResult = vi.fn(async () => {});
  const warning = vi.spyOn(console, 'warn').mockImplementation(() => {});
  const extension = implementationExtension(ctx, f.settings, async () => {}, tasks, {
    execute: f.execute,
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', { patch });
      await call('preparePullRequest', proposal);
    }),
    onPublished: async () => {
      throw new Error('Chat post failed');
    },
    onCiResult
  });
  const call = await workerTools({
    cwd: f.settings.directory,
    model: 'test/model',
    extensions: [extension]
  });
  try {
    const handle = JSON.parse(
      (await call('implementChatto', { request: 'Fix', announcement: 'Starting' })).content[0]!
        .text!
    );
    await vi.waitFor(() => expect(tasks.get(handle.id).status).toBe('completed'));
    expect(onCiResult).toHaveBeenCalledOnce();
    expect(JSON.parse(tasks.get(handle.id).result!)).toMatchObject({
      outcome: 'completed',
      publicationNoticeDelivered: false,
      ciNoticeDelivered: true,
      noticeDelivered: false
    });
  } finally {
    warning.mockRestore();
    await tasks.dispose();
  }
});

test('an unexpected worker error produces one safe stopped result for the owner', async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx, { notifyActivity: false });
  const onStopped = vi.fn(async () => {});
  const extension = implementationExtension(ctx, f.settings, async () => {}, tasks, {
    execute: f.execute,
    onStopped,
    createAgent: async () => ({
      dispose() {},
      async runOutcome() {
        throw new Error('private provider detail');
      }
    })
  });
  const call = await workerTools({
    cwd: f.settings.directory,
    model: 'test/model',
    extensions: [extension]
  });
  try {
    const handle = JSON.parse(
      (await call('implementChatto', { request: 'Fix', announcement: 'Starting' })).content[0]!
        .text!
    );
    await vi.waitFor(() => expect(tasks.get(handle.id).status).toBe('completed'));
    expect(onStopped).toHaveBeenCalledOnce();
    expect(JSON.stringify(tasks.get(handle.id))).not.toContain('private provider detail');
    expect(JSON.parse(tasks.get(handle.id).result!)).toMatchObject({
      outcome: 'blocked',
      noticeDelivered: true
    });
  } finally {
    await tasks.dispose();
  }
});

test('notification refusal says no implementation started and has no side effects', async () => {
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx);
  const announce = vi.fn();
  const onBlocked = vi.fn();
  const extension = implementationExtension(
    ctx,
    { directory: '/does-not-exist', repository: 'example/chatto' },
    announce,
    tasks,
    { requestVersion: () => undefined, onBlocked }
  );
  const call = await workerTools({ cwd: '/unused', model: 'test/model', extensions: [extension] });
  try {
    const result = JSON.parse(
      (
        await call('implementChatto', {
          request: 'Fix',
          announcement: 'Starting now',
          investigationId: 'missing'
        })
      ).content[0]!.text!
    );
    expect(result).toMatchObject({
      outcome: 'blocked',
      summary: expect.stringContaining('Implementation was not started')
    });
    expect(onBlocked).toHaveBeenCalledExactlyOnceWith(result.summary);
    expect(announce).not.toHaveBeenCalled();
    expect(tasks.list()).toEqual([]);
  } finally {
    await tasks.dispose();
  }
});

test.each(['response-lost', 'not-created', 'wrong-url', 'wrong-commit'])(
  'handles publication uncertainty: %s',
  async (mode) => {
    const f = await fixture();
    const execute: ImplementationProcess = async (command, args, options) => {
      if (command === 'gh' && args[1] === 'create') {
        if (mode === 'response-lost') await f.execute(command, args, options);
        throw new Error('Publication response lost');
      }
      if (command === 'gh' && args[1] === 'view' && mode === 'wrong-url')
        return JSON.stringify({
          url: 'https://example.invalid/pull/7',
          headRefName: args[2],
          baseRefName: 'main',
          state: 'OPEN'
        });
      if (command === 'gh' && args[1] === 'view' && mode === 'wrong-commit')
        return JSON.stringify({
          url: 'https://github.com/example/chatto/pull/7',
          headRefName: args[2],
          headRefOid: 'wrong',
          baseRefName: 'main',
          state: 'OPEN'
        });
      return f.execute(command, args, options);
    };
    const result = await createImplementation(f.settings, {
      execute,
      createAgent: worker(async (_options, call) => {
        await call('apply_patch', { patch });
        await call('preparePullRequest', proposal);
      })
    })(createWorkflowContext(), { request: 'Fix' });
    expect(result.outcome).toBe(mode === 'response-lost' ? 'completed' : 'publication_unknown');
    expect(result.prUrl).toBe(
      mode === 'response-lost' ? 'https://github.com/example/chatto/pull/7' : undefined
    );
  }
);

test('cancellation stops work, disposes the worker and retains the patch without publication', async () => {
  const f = await fixture();
  const controller = new AbortController();
  const dispose = vi.fn();
  await expect(
    createImplementation(f.settings, {
      execute: f.execute,
      createAgent: async (options) => ({
        dispose,
        async runOutcome() {
          await (
            await workerTools(options)
          )('apply_patch', { patch });
          controller.abort();
          return { outcome: 'completed', summary: 'Cancelled', usage: emptyTokenUsage() };
        }
      })
    })({ ...createWorkflowContext(), signal: controller.signal }, { request: 'Fix' })
  ).rejects.toThrow();
  expect(dispose).toHaveBeenCalledOnce();
  expect(f.calls.some((call) => call.args.includes('push'))).toBe(false);
  const [folder] = await readdir(f.settings.artifactsDirectory);
  expect(
    await readFile(join(f.settings.artifactsDirectory, folder!, 'changes.patch'), 'utf8')
  ).toContain('+fixed');
  expect(
    JSON.parse(
      await readFile(join(f.settings.artifactsDirectory, folder!, 'metadata.json'), 'utf8')
    )
  ).toMatchObject({ stage: 'interrupted' });
});

test('settings are opt-in and remote matching cannot select a different host or repository', () => {
  vi.stubEnv('CHATTO_IMPLEMENTATION_REPOSITORY', '');
  expect(implementationSettings()).toBeUndefined();
  vi.stubEnv('CHATTO_IMPLEMENTATION_REPOSITORY', 'example/chatto');
  vi.stubEnv('CHATTO_SOURCE_DIRECTORY', '');
  expect(() => implementationSettings()).toThrow('CHATTO_SOURCE_DIRECTORY');
  vi.stubEnv('CHATTO_SOURCE_DIRECTORY', '/repo');
  vi.stubEnv('CHATTO_SOURCE_REF', undefined);
  expect(implementationSettings()).toMatchObject({
    directory: '/repo',
    repository: 'example/chatto',
    baseBranch: 'main'
  });
  vi.stubEnv('CHATTO_SOURCE_REF', 'origin/develop');
  expect(implementationSettings()?.baseBranch).toBe('origin/develop');
  expect(matchesRepository('git@github.com:example/chatto.git', 'example/chatto')).toBe(true);
  for (const remote of [
    'https://github.com/other/chatto',
    'https://github.com.evil/example/chatto',
    'https://token@github.com/example/chatto'
  ])
    expect(matchesRepository(remote, 'example/chatto')).toBe(false);
  expect(() => createImplementation({ directory: '/missing', repository: '--help' })).toThrow(
    'owner/repo'
  );
});

test('provider login failure reaches the owner without claiming edits or publication', async () => {
  const f = await fixture();
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    createAgent: async () => ({
      dispose() {},
      async runOutcome() {
        return {
          outcome: 'failed',
          failureReason: 'provider_error',
          summary:
            'The model provider login has expired. Sign in again on the agent host before retrying.',
          usage: emptyTokenUsage()
        };
      }
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result).toMatchObject({
    outcome: 'blocked',
    summary: expect.stringContaining('login has expired'),
    checks: []
  });
  expect(result.prUrl).toBeUndefined();
  expect(f.calls.some((call) => call.args.includes('push'))).toBe(false);
});

test('unconsumed steering prevents publication', async () => {
  const f = await fixture();
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', { patch });
      await call('preparePullRequest', proposal);
    })
  })(
    {
      ...createWorkflowContext(),
      inbox: (async function* () {
        yield 'Do not publish yet';
      })()
    },
    { request: 'Fix' }
  );
  expect(result).toMatchObject({
    outcome: 'blocked',
    summary: expect.stringContaining('clarification was not consumed')
  });
  expect(f.calls.some((call) => call.args.includes('push'))).toBe(false);
});

test('a wrong origin fails before worker creation or any fetch', async () => {
  const f = await fixture();
  const createAgent = vi.fn();
  const result = await createImplementation(f.settings, {
    createAgent,
    execute: async (command, args, options) => {
      if (args.includes('get-url')) return 'https://github.com/other/repo.git';
      return f.execute(command, args, options);
    }
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result).toMatchObject({
    outcome: 'blocked',
    summary: expect.stringContaining('origin matches')
  });
  expect(createAgent).not.toHaveBeenCalled();
  expect(f.calls.some((call) => call.args.includes('fetch'))).toBe(false);
});

test.each(['origin/main', 'refs/remotes/origin/main'])(
  'accepts remote-tracking base %s',
  async (baseBranch) => {
    const f = await fixture();
    const createAgent = vi.fn(worker(async () => {}, 'blocked'));
    await createImplementation({ ...f.settings, baseBranch }, { execute: f.execute, createAgent })(
      createWorkflowContext(),
      { request: 'Fix' }
    );
    expect(createAgent).toHaveBeenCalledOnce();
    expect(f.calls.find((call) => call.args.includes('fetch'))?.args).toContain(
      'refs/heads/main:refs/remotes/origin/main'
    );
  }
);

test.each(['auth', 'fetch'])(
  'reports a safe %s preflight failure without starting the worker',
  async (stage) => {
    const f = await fixture();
    const createAgent = vi.fn();
    const result = await createImplementation(f.settings, {
      createAgent,
      execute: async (command, args, options) => {
        if (args.includes(stage)) throw new Error('private credentials and subprocess output');
        return f.execute(command, args, options);
      }
    })(createWorkflowContext(), { request: 'Fix' });
    expect(result).toMatchObject({
      outcome: 'blocked',
      summary: expect.stringContaining(stage === 'auth' ? 'authentication check' : 'base branch'),
      checks: []
    });
    expect(JSON.stringify(result)).not.toContain('private credentials');
    expect(createAgent).not.toHaveBeenCalled();
  }
);

test('the tool waits for its announcement, returns a handle, accepts steering, and reports the PR', async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx);
  const notices = tasks.notifications[Symbol.asyncIterator]();
  const delivered = Promise.withResolvers<void>();
  const started = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<void>();
  const announce = vi.fn(async () => delivered.promise);
  const steer = vi.fn(async () => true);
  const plan = {
    baseCommit: await f.git('rev-parse', 'HEAD'),
    goal: 'Fix the value',
    steps: [{ files: ['example.txt'], change: 'Replace original with fixed' }],
    acceptanceCriteria: ['Value is fixed'],
    checks: ['Check value'],
    openQuestions: []
  };
  const createAgent = vi.fn(async (options: AgentOptions) => ({
    steer,
    dispose: () => {},
    async runOutcome(_ctx: unknown, prompt: string) {
      expect(JSON.parse(prompt).plan).toEqual(plan);
      started.resolve();
      await finish.promise;
      const call = await workerTools(options);
      await call('apply_patch', { patch });
      await call('preparePullRequest', proposal);
      return { outcome: 'completed' as const, summary: 'Done', usage: emptyTokenUsage() };
    }
  }));
  const extension = implementationExtension(ctx, f.settings, announce, tasks, {
    execute: f.execute,
    createAgent,
    plans: new Map([['investigation', plan]])
  });
  const call = await workerTools({
    cwd: f.settings.directory,
    model: 'test/model',
    extensions: [extension]
  });
  try {
    await expect(
      call('implementChatto', {
        request: 'Fix the value',
        investigationId: 'unknown',
        announcement: 'Starting'
      })
    ).rejects.toThrow('No completed implementation plan');
    expect(announce).not.toHaveBeenCalled();
    const pending = call('implementChatto', {
      request: 'Fix the value',
      investigationId: 'investigation',
      announcement: "I'll implement the fix and run its checks."
    });
    await vi.waitFor(() => expect(announce).toHaveBeenCalledOnce());
    expect(createAgent).not.toHaveBeenCalled();
    expect(f.calls).toEqual([]);
    delivered.resolve();
    const handle = JSON.parse((await pending).content[0]!.text!);
    expect(handle.status).toBe('running');
    await started.promise;
    await tasks.send(handle.id, 'Keep the public API unchanged');
    await vi.waitFor(() => expect(steer).toHaveBeenCalledWith('Keep the public API unchanged'));
    finish.resolve();
    const notification = JSON.parse((await notices.next()).value!);
    expect(notification.type).toBe('task.completed');
    expect(JSON.parse(notification.task.result)).toMatchObject({
      outcome: 'completed',
      prUrl: 'https://github.com/example/chatto/pull/7'
    });
    expect(announce).toHaveBeenCalledOnce();
  } finally {
    delivered.resolve();
    finish.resolve();
    await tasks.dispose();
  }
});

test('validation diagnostics retain failure detail without credentials, URLs, or unbounded output', () => {
  vi.stubEnv('OPENROUTER_API_KEY', 'private-api-value');
  const diagnostic = validationDiagnostic(
    'x'.repeat(10_000) +
      '\nFAIL navigation.spec.ts: expected 1, received 0\n/worktree/file.ts private-api-value person@example.invalid https://example.invalid/?token=abc Bearer secret',
    '/worktree'
  );
  expect(diagnostic).toContain('FAIL navigation.spec.ts: expected 1, received 0');
  expect(diagnostic).toContain('<worktree>/file.ts');
  expect(diagnostic).not.toMatch(/private-api-value|person@|token=abc|Bearer secret/);
  expect(diagnostic.length).toBeLessThanOrEqual(8000);
});

test('worker stop reasons are bounded and redacted before reaching the owner', () => {
  vi.stubEnv('OPENAI_API_KEY', 'private-api-value');
  const reason = workerStopReason(
    '18 catalog sections remain. /Users/test/private/file.ts private-api-value person@example.invalid https://example.invalid/?token=abc Bearer secret ' +
      'x'.repeat(2000),
    '/worktree'
  );
  expect(reason).toContain('18 catalog sections remain');
  expect(reason).not.toMatch(/\/Users\/test|private-api-value|person@|token=abc|Bearer secret/);
  expect(reason.length).toBeLessThanOrEqual(800);
});

test.skipIf(!process.env.CHATTO_EVAL_MODEL)(
  'live worker edits a fixture and passes real host setup and validation',
  async () => {
    const f = await fixture();
    await writeFile(join(f.settings.directory, '.gitignore'), 'node_modules/\n');
    await writeFile(join(f.settings.directory, '.tool-versions'), 'node 24.21.0\npnpm 11.25.0\n');
    await writeFile(
      join(f.settings.directory, 'package.json'),
      JSON.stringify({
        name: 'implementation-fixture',
        private: true,
        scripts: { check: 'node --check regression.cjs', test: 'node --test regression.cjs' }
      })
    );
    await writeFile(
      join(f.settings.directory, 'regression.cjs'),
      "const {test}=require('node:test'); const {strictEqual}=require('node:assert'); const {readFileSync}=require('node:fs'); test('fixed value',()=>strictEqual(readFileSync('example.txt','utf8'),'fixed\\n'));\n"
    );
    await implementationProcess(
      'mise',
      ['x', '--', 'pnpm', 'install', '--lockfile-only', '--ignore-scripts'],
      { cwd: f.settings.directory, signal: AbortSignal.timeout(30_000), captureDiagnostics: true }
    ).catch((error) => {
      throw new Error(
        error instanceof ImplementationCommandError ? error.output : 'Fixture setup failed'
      );
    });
    await f.git('add', '.');
    await f.git('-c', 'commit.gpgsign=false', 'commit', '-m', 'test: add fixture validation');
    await f.git('push', 'origin', 'main');
    const result = await createImplementation(
      { ...f.settings, model: process.env.CHATTO_EVAL_MODEL },
      {
        execute: (command, args, options) =>
          command === 'mise'
            ? implementationProcess(command, args, options)
            : f.execute(command, args, options)
      }
    )(
      { ...createWorkflowContext(), signal: AbortSignal.timeout(120_000) },
      {
        request:
          'Fix example.txt: its complete contents must be fixed followed by one newline. The existing regression test defines the correct behavior; do not weaken it. Prepare the PR when the edit is ready. Host validation runs after your report.'
      }
    );
    expect(result.outcome).toBe('completed');
    expect(result.checks).toHaveLength(2);
    expect(result.checks.every((check) => check.passed)).toBe(true);
    expect(result.prUrl).toBe('https://github.com/example/chatto/pull/7');
    expect(await readFile(join(f.settings.directory, 'example.txt'), 'utf8')).toBe('original\n');
  },
  160_000
);
