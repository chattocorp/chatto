import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, readFile, writeFile, rm, readdir, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import { createWorkflowContext, emptyTokenUsage } from 'runling';
import type { AgentExtensionAPI, AgentOptions, AgentRunOptions } from 'runling/agents';
import { createAgentTasks, createApprovalQueue } from 'runling/agents';
import {
  createImplementation,
  implementationCommandEnvKeys,
  implementationExtension,
  implementationSettings,
  matchesRepository,
  validationDiagnostic,
  workerStopReason
} from './implement.ts';
import { ConfigurationError } from '../settings.ts';
import { taskContext, userFacingTaskNotifications } from './task-context.ts';
import { listResumableArtifacts } from './implementation-artifacts.ts';
import { failedHunkLines } from './implementation-tools.ts';
import {
  HostCommandError,
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
      // Setup and formatting are host preparation, not checks.
      if (
        args.includes('install') ||
        args.includes('turbo') ||
        args.includes('prettier') ||
        args.includes('gofmt')
      )
        return '';
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
  const hooks: ((event: {
    type: 'tool_call';
    toolName: string;
    input: object;
  }) => Promise<{ block?: boolean; reason?: string } | undefined>)[] = [];
  const tools = await registeredTools(options, hooks);
  return async (name: string, input: object) => {
    for (const hook of hooks) {
      const decision = await hook({ type: 'tool_call', toolName: name, input });
      if (decision?.block) throw new Error(decision.reason);
    }
    return tools.get(name)!.execute('call', input as never);
  };
}
async function registeredTools(
  options: AgentOptions,
  hooks: ((event: {
    type: 'tool_call';
    toolName: string;
    input: object;
  }) => Promise<{ block?: boolean; reason?: string } | undefined>)[] = []
) {
  const tools = new Map<string, Tool>();
  for (const extension of options.extensions ?? []) {
    const factory = typeof extension === 'function' ? extension : extension.factory;
    await factory({
      on(name: string, handler: (typeof hooks)[number]) {
        if (name === 'tool_call') hooks.push(handler);
      },
      registerTool(tool: Tool) {
        tools.set(tool.name, tool);
      }
    } as unknown as AgentExtensionAPI);
  }
  return tools;
}
const patch =
  'diff --git a/example.txt b/example.txt\n--- a/example.txt\n+++ b/example.txt\n@@ -1 +1 @@\n-original\n+fixed\n';
const check = 'test "$(cat example.txt)" = fixed';
const proposal = {
  title: 'fix(example): correct the value',
  summary: 'Correct the value to fix the reported behavior.',
  notes: ['Browser behavior was not checked.']
};

test.each(['allow', 'deny', 'changed'] as const)(
  'publication waits for its owner: %s',
  async (outcome) => {
    const f = await fixture();
    const root = createWorkflowContext();
    const queue = createApprovalQueue({ signal: root.signal });
    let worktree = '';
    const implement = createImplementation(f.settings, {
      execute: f.execute,
      requestApproval: (ctx, action) => queue.request(action, { signal: ctx.signal, notify() {} }),
      createAgent: worker(async (options, call) => {
        worktree = options.cwd;
        await call('apply_patch', { patch });
        await call('preparePullRequest', proposal);
      })
    });
    const running = implement(root, { request: 'Fix the value' });
    await vi.waitFor(() => expect(queue.list()[0]?.action).toBe('prepare_pull_request'));
    const [prepare] = queue.list();
    queue.decide(prepare!.id, { decision: 'allow', reason: 'The proposal matches the request.' });
    await vi.waitFor(() => expect(queue.list()[0]?.action).toBe('publish_pull_request'));
    expect(f.calls.some(({ args }) => args.includes('push') || args.includes('create'))).toBe(
      false
    );
    const [publication] = queue.list();
    expect(publication!.details).toMatchObject({
      repository: 'example/chatto',
      proposal,
      tree: expect.any(String)
    });
    if (outcome === 'changed')
      await writeFile(join(worktree, 'example.txt'), 'changed while waiting\n');
    queue.decide(publication!.id, {
      decision: outcome === 'deny' ? 'deny' : 'allow',
      reason: 'Owner decision.'
    });
    const result = await running;
    expect(result.outcome).toBe(outcome === 'allow' ? 'completed' : 'blocked');
    expect(f.calls.some(({ args }) => args.includes('push'))).toBe(outcome === 'allow');
    queue.dispose();
  }
);
/** A root context that collects the progress notices a task sends to its parent. Milestone
 * notices, which carry `data`, are left out. */
function noticeContext(notices: string[]) {
  return {
    ...createWorkflowContext(),
    emit: async (value: unknown) => {
      const update = value as { type?: unknown; text?: unknown; data?: unknown };
      if (update?.type === 'notice' && update.data === undefined) notices.push(String(update.text));
    }
  };
}

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
      expect(options.codemode).toEqual({ timeoutMs: 1_200_000 });
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
      { command: 'mise x -- pnpm run lint', passed: true }
    ],
    ci: { status: 'passed', passed: 1, repairs: 0 }
  });
  expect(f.body).toContain('## Verification');
  expect(f.body).toContain('Passed locally: mise x -- pnpm run lint');
  expect(f.body).toContain('Tests run in CI');
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
    activity: 'Validating · mise x -- pnpm run lint',
    value: {
      phase: 'validating',
      currentCheck: 'mise x -- pnpm run lint',
      completedChecks: ['mise x -- pnpm run check'],
      pendingChecks: ['mise x -- pnpm run lint'],
      artifactId: result.artifactId
    }
  });
  expect(updates).toContainEqual({
    type: 'state',
    value: {
      phase: 'published',
      prUrl: result.prUrl,
      completedChecks: ['mise x -- pnpm run check', 'mise x -- pnpm run lint'],
      pendingChecks: [],
      artifactId: result.artifactId
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

test('worker can inspect one changed file and run selected frontend and Go tests', async () => {
  const f = await fixture();
  const lintGate = Promise.withResolvers<void>();
  // SvelteKit route folders contain brackets.
  const spec = 'src/routes/[serverId]/example.test.ts';
  await mkdir(join(f.settings.directory, 'apps/frontend/src/routes/[serverId]'), {
    recursive: true
  });
  await mkdir(join(f.settings.directory, 'cli/internal/core'), { recursive: true });
  await writeFile(join(f.settings.directory, 'cli/internal/core/core.go'), 'package core\n');
  await writeFile(join(f.settings.directory, 'apps/frontend', spec), 'test fixture\n');
  await f.git('add', '.');
  await f.git('-c', 'commit.gpgsign=false', 'commit', '-m', 'Add test fixture');
  await f.git('push', 'origin', 'main');
  const result = await createImplementation(f.settings, {
    // lint-cli waits until the test releases it, so another check can start meanwhile.
    execute: async (command, args, options) => {
      if (args.at(-1) === 'lint-cli') await lintGate.promise;
      return f.execute(command, args, options);
    },
    createAgent: worker(async (options, call) => {
      await call('apply_patch', { patch });
      // A failed patch shows the current lines around its hunk.
      const stale = await call('apply_patch', { patch });
      expect(stale.content[0]?.text).toContain('Current lines 1-2 of example.txt:\n1: fixed');
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
      for (const packages of [['./...'], ['../cli'], ['./internal/missing'], ['-exec=sh']])
        expect((await call('runGoTests', { packages })).content[0]?.text).toContain(
          'Select existing Go package directories'
        );
      expect(f.calls).toHaveLength(before + 1);
      // A Go test does not copy files while another check runs.
      const lint = call('runCheck', { check: 'lint-cli' });
      expect(
        (await call('runGoTests', { packages: ['./internal/core'] })).content[0]?.text
      ).toContain('already running');
      expect(f.calls).toHaveLength(before + 1);
      lintGate.resolve();
      await lint;
      expect(
        (await call('runGoTests', { packages: ['./internal/core/...'], run: 'TestCore' }))
          .content[0]?.text
      ).toContain('Passed: mise x -- go -C cli test');
      // `cmd` embeds legal files that the sync copies first.
      expect(f.calls.at(-2)).toEqual({ command: 'mise', args: ['run', 'sync-cli-legal'] });
      // Complete test suites run in CI, not in the worker.
      const runCheck = (await registeredTools(options)).get('runCheck') as unknown as {
        parameters: { properties: { check: { anyOf: { const: string }[] } } };
      };
      expect(runCheck.parameters.properties.check.anyOf.map((option) => option.const)).toEqual([
        'check',
        'lint',
        'check:frontend',
        'lint:frontend',
        'build:frontend',
        'lint-cli'
      ]);
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
    { command: 'mise run lint-cli', passed: true },
    {
      command:
        'mise x -- go -C cli test -trimpath -p 4 -tags test_endpoints -run=TestCore ./internal/core/...',
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
      value: { phase: 'editing_checkpoint', idleCheckpoints: 0, artifactId: result.artifactId }
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
      expect(check.content[0]?.text).not.toContain('test@example.invalid');
      expect(check.content[0]?.text).not.toContain('https://example.invalid');
      await call('preparePullRequest', proposal);
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
  expect(result.workerChecks).toEqual([{ command: 'mise x -- pnpm run check', passed: false }]);
  expect(result.checks.every((check) => check.passed)).toBe(true);
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
      return f.execute(command, args, options);
    }
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
  expect(createAgent).toHaveBeenCalledOnce();
  expect(turns).toBe(2);
  expect(result.checks.every((check) => check.passed)).toBe(true);
});

test('worker progress updates reach the user, redacted and at most once per interval', async () => {
  const f = await fixture();
  const updates: string[] = [];
  const results: (string | undefined)[] = [];
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    progressTiming: { minIntervalMs: 0, quietMs: 60_000 },
    createAgent: worker(async (_options, call) => {
      results.push(
        (
          await call('reportProgress', {
            message: 'Adding the command; see https://example.invalid'
          })
        ).content[0]?.text
      );
      await call('apply_patch', { patch });
      await call('preparePullRequest', proposal);
    })
  })(noticeContext(updates), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
  expect(results).toEqual(['Sent.']);
  // The task sends notices only to its parent, which decides what reaches the user.
  expect(updates).toEqual(['Adding the command; see [url]']);

  const held: string[] = [];
  const texts: (string | undefined)[] = [];
  await createImplementation(f.settings, {
    execute: f.execute,
    progressTiming: { minIntervalMs: 2_000, quietMs: 60_000 },
    createAgent: worker(async (_options, call) => {
      // Both arrive before the interval ends; only the newer one is posted, once it ends.
      texts.push(
        (await call('reportProgress', { message: 'Reading the composer' })).content[0]?.text
      );
      await call('reportProgress', { message: 'Adding the command' });
      expect(held).toEqual([]);
      await vi.waitFor(() => expect(held).toEqual(['Adding the command']), { timeout: 4_000 });
      await call('apply_patch', { patch });
      await call('preparePullRequest', proposal);
    })
  })(noticeContext(held), { request: 'Fix' });
  expect(texts[0]).toMatch(/^Queued: the host posts it in about/);
  expect(held).toEqual(['Adding the command']);
});

test('after a quiet period the host posts what it knows about the worker progress', async () => {
  const f = await fixture();
  const updates: string[] = [];
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    progressTiming: { quietMs: 20, checkMs: 5 },
    createAgent: worker(async (_options, call) => {
      await vi.waitFor(() =>
        expect(updates[0]).toBe('Still reading the code. No files have changed yet.')
      );
      await call('apply_patch', { patch });
      await vi.waitFor(() =>
        expect(updates.at(-1)).toBe('Still working on the change: 1 changed file so far.')
      );
      await call('preparePullRequest', proposal);
    })
  })(noticeContext(updates), { request: 'Fix' });
  expect(result.outcome).toBe('completed');
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
  expect(
    f.calls.some(
      (call) =>
        call.command === 'mise' && !call.args.includes('install') && !call.args.includes('turbo')
    )
  ).toBe(false);
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
  // Applied patches are transient; only the retained diff remains.
  expect(
    (await readdir(join(first.worktree, '..'))).filter((name) => name.startsWith('edit-'))
  ).toEqual([]);
  const resumed = await createImplementation(f.settings, {
    execute: f.execute,
    ownerKey,
    createAgent: async (options) => ({
      dispose() {},
      async runOutcome(_ctx: unknown, prompt: string) {
        expect(JSON.parse(prompt).handoff).toMatchObject({
          nextSteps: ['Review the diff and prepare the PR']
        });
        expect(JSON.parse(prompt)).toMatchObject({
          request: 'Fix the value',
          followUps: [{ request: 'Continue', context: 'An unreviewed draft is fine.' }]
        });
        expect(await readFile(join(options.cwd, 'example.txt'), 'utf8')).toBe('fixed\n');
        const call = await workerTools(options);
        await call('preparePullRequest', proposal);
        return { outcome: 'completed' as const, summary: 'Ready', usage: emptyTokenUsage() };
      }
    })
  })(createWorkflowContext(), {
    request: 'Continue',
    context: 'An unreviewed draft is fine.',
    resumeArtifactId: first.artifactId
  });
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
    input: { request: 'Fix the value' },
    followUps: [{ request: 'Continue', context: 'An unreviewed draft is fine.' }]
  });
});

test('a continued implementation reopens the worker conversation and lists as resumable', async () => {
  const f = await fixture();
  const ownerKey = 'conversation-owner';
  const sessionFiles: (string | undefined)[] = [];
  const first = await createImplementation(f.settings, {
    execute: f.execute,
    ownerKey,
    createAgent: async (options) => {
      sessionFiles.push(options.sessionFile);
      return {
        dispose() {},
        async runOutcome() {
          await (
            await workerTools(options)
          )('apply_patch', { patch });
          // Stand in for Pi, which writes the conversation after the first model reply.
          await writeFile(options.sessionFile!, '{"type":"session"}\n');
          return { outcome: 'blocked' as const, summary: 'Stopped', usage: emptyTokenUsage() };
        }
      };
    }
  })(createWorkflowContext(), { request: 'Fix the value' });
  expect(sessionFiles).toEqual([join(first.worktree, '..', 'worker-session.jsonl')]);
  const expected = { ownerKey, repository: 'example/chatto', baseBranch: 'main' };
  expect(await listResumableArtifacts(f.settings.artifactsDirectory, expected)).toEqual([
    {
      artifactId: first.artifactId,
      request: 'Fix the value',
      stage: 'blocked',
      updatedAt: expect.any(Number),
      sessionSaved: true
    }
  ]);
  expect(
    await listResumableArtifacts(f.settings.artifactsDirectory, { ...expected, ownerKey: 'other' })
  ).toEqual([]);

  let prompt: Record<string, unknown> | undefined;
  const resumed = await createImplementation(f.settings, {
    execute: f.execute,
    ownerKey,
    createAgent: async (options) => ({
      dispose() {},
      async runOutcome(_ctx: unknown, text: string) {
        prompt = JSON.parse(text);
        await (
          await workerTools(options)
        )('preparePullRequest', proposal);
        return { outcome: 'completed' as const, summary: 'Ready', usage: emptyTokenUsage() };
      }
    })
  })(createWorkflowContext(), {
    request: 'Continue',
    resumeArtifactId: first.artifactId
  });
  expect(resumed.outcome).toBe('completed');
  // The saved conversation already holds the request; the prompt only resumes it.
  expect(prompt).toEqual({
    resumeArtifactId: first.artifactId,
    followUps: [{ request: 'Continue' }],
    continuation: expect.stringContaining('Your conversation so far is above.')
  });
  expect(await listResumableArtifacts(f.settings.artifactsDirectory, expected)).toEqual([]);
});

test('git retries while another process holds the index lock, and a host failure names its command', async () => {
  const f = await fixture();
  const locked = (message: string) => {
    const error = new HostCommandError(message);
    error.lockHeld = true;
    return error;
  };
  let addAttempts = 0;
  const retried = await createImplementation(f.settings, {
    execute: async (command, args, options) => {
      if (command === 'git' && args.includes('add') && ++addAttempts <= 2)
        throw locked('git add failed (exit 128)');
      return f.execute(command, args, options);
    },
    createAgent: worker(async (_options, call) => {
      await call('apply_patch', { patch });
      await call('preparePullRequest', proposal);
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(retried.outcome).toBe('completed');
  expect(addAttempts).toBeGreaterThan(2);

  const failed = await createImplementation(f.settings, {
    execute: async (command, args, options) => {
      if (command === 'git' && args.includes('write-tree'))
        throw new HostCommandError('git write-tree failed (exit 128)');
      return f.execute(command, args, options);
    },
    createAgent: worker(async () => {})
  })(createWorkflowContext(), { request: 'Fix' });
  expect(failed).toMatchObject({
    outcome: 'blocked',
    summary:
      'The implementation stopped during setup because git write-tree failed (exit 128). The work so far is kept.'
  });
});

test('a cancelled implementation keeps its artifact ID in the supervisor snapshot', async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx, { notifyActivity: false });
  const editing = Promise.withResolvers<void>();
  const extension = implementationExtension(ctx, f.settings, async () => {}, tasks, {
    execute: f.execute,
    ownerKey: 'owner',
    createAgent: async () => ({
      dispose() {},
      async runOutcome(workerCtx: { signal: AbortSignal }) {
        editing.resolve();
        await new Promise((_resolve, reject) =>
          workerCtx.signal.addEventListener('abort', () => reject(workerCtx.signal.reason), {
            once: true
          })
        );
        throw new Error('unreachable');
      }
    })
  });
  const call = await workerTools({
    cwd: f.settings.directory,
    model: 'm',
    extensions: [extension]
  });
  try {
    const handle = JSON.parse(
      (await call('implementChatto', { request: 'Fix', announcement: 'Starting' })).content[0]!
        .text!
    );
    await editing.promise;
    tasks.cancel(handle.id);
    await vi.waitFor(() => expect(tasks.get(handle.id).status).toBe('cancelled'));
    const [snapshot] = taskContext(tasks.list());
    expect(snapshot).toMatchObject({
      status: 'cancelled',
      state: { artifactId: expect.stringMatching(/^implementation-/) }
    });
    expect(Object.keys(snapshot!.state!)).toEqual(['artifactId']);
  } finally {
    await tasks.dispose();
  }
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
  // Setup, the dependency build, the worker check, formatting, and two final checks.
  expect(checkEnvironments).toHaveLength(6);
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
  {
    path: 'apps/frontend/example.txt',
    scripts: ['check:frontend', 'lint:frontend'],
    formatters: ['prettier']
  },
  {
    path: 'cli/example.go',
    scripts: ['check', 'lint', 'lint-cli'],
    formatters: ['prettier', 'gofmt']
  },
  {
    path: 'proto/chatto/example.proto',
    scripts: ['codegen-proto', 'check', 'lint', 'lint-proto'],
    formatters: ['prettier']
  }
])('host prepares and validates $path', async ({ path, scripts, formatters }) => {
  const f = await fixture();
  const validation: string[] = [];
  const formatted: string[] = [];
  const result = await createImplementation(f.settings, {
    execute: async (command, args, options) => {
      if (command === 'mise' && (args.includes('prettier') || args.includes('gofmt'))) {
        expect(args.at(-1)).toBe(path);
        formatted.push(args.includes('prettier') ? 'prettier' : 'gofmt');
        return '';
      }
      if (command === 'mise' && !args.includes('install') && !args.includes('turbo')) {
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
  expect(formatted).toEqual(formatters);
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
    expect(result.summary).toContain('three attempts');
    if (mode === 'failed-check')
      expect(result.checks).toEqual([
        { command: 'mise x -- pnpm run check', passed: false, diagnostic: 'Assertion failed' }
      ]);
    expect(turns).toBe(3);
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

test('one request gets one implementation attempt; a new human request can retry', async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx);
  let version = 1;
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

test('a blocked worker reason and its check count reach the owner without claiming final validation', async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx, { notifyActivity: false });
  const extension = implementationExtension(ctx, f.settings, async () => {}, tasks, {
    execute: f.execute,
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
    const result = JSON.parse(tasks.get(handle.id).result!);
    expect(result).toMatchObject({
      outcome: 'blocked',
      checks: [],
      workerChecks: [{ command: 'mise x -- pnpm run check', passed: true }]
    });
    // The supervisor reports the result in its own words from these facts.
    expect(result.summary).toContain('18 catalog sections remain');
    expect(result.artifactId).toMatch(/^implementation-/);
    expect(
      f.calls.filter((entry) => entry.command === 'mise' && entry.args.at(-1) === 'check')
    ).toHaveLength(1);
  } finally {
    await tasks.dispose();
  }
});

const passedChecks = {
  status: 'passed' as const,
  passed: 3,
  failed: 0,
  pending: 0,
  skipped: 0,
  failures: []
};
const failedChecks = {
  status: 'failed' as const,
  passed: 1,
  failed: 1,
  pending: 1,
  skipped: 0,
  failures: [
    { name: 'test-frontend-unit', link: 'https://github.com/example/chatto/actions/runs/12/job/34' }
  ]
};
const regressionPatch = '--- /dev/null\n+++ b/regression.txt\n@@ -0,0 +1 @@\n+covered\n';
const remoteSubjects = async (f: Awaited<ReturnType<typeof fixture>>, branch: string) =>
  (await exec('git', ['--git-dir', f.remote, 'log', '--format=%s', branch])).stdout
    .trim()
    .split('\n');

test('the supervisor hears each milestone; a CI failure and its job log go to the same worker, which pushes a fix', async () => {
  const f = await fixture();
  const approvedActions: string[] = [];
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx, { notifyActivity: false });
  const observed: { headCommit: string; stopOnFailure?: boolean }[] = [];
  const observeChecks = vi.fn(async (options: { headCommit: string; stopOnFailure?: boolean }) => {
    observed.push({ headCommit: options.headCommit, stopOnFailure: options.stopOnFailure });
    return observed.length === 1 ? failedChecks : passedChecks;
  });
  const prompts: string[] = [];
  const createAgent = vi.fn(async (options: AgentOptions) => ({
    dispose: vi.fn(),
    async runOutcome(_ctx: unknown, prompt: string) {
      prompts.push(prompt);
      const call = await workerTools(options);
      if (prompts.length === 1) {
        await call('apply_patch', { patch });
        await call('preparePullRequest', proposal);
      } else await call('apply_patch', { patch: regressionPatch });
      return { outcome: 'completed' as const, summary: 'Ready', usage: emptyTokenUsage() };
    }
  }));
  const extension = implementationExtension(ctx, f.settings, async () => {}, tasks, {
    requestApproval: async (_ctx, action) => {
      approvedActions.push(action.action);
      return { decision: 'allow', reason: 'Within the delegated fix.' };
    },
    execute: async (command, args, options) =>
      command === 'gh' && args[0] === 'api'
        ? '2026-01-01T00:00:00.0Z FAIL src/a.spec.ts\n2026-01-01T00:00:00.1Z ##[error]Process completed with exit code 1.'
        : f.execute(command, args, options),
    createAgent,
    observeChecks
  });
  const call = await workerTools({
    cwd: f.settings.directory,
    model: 'test/model',
    extensions: [extension]
  });
  // Read notifications like the supervisor conversation does.
  const woken: { type: string; data?: Record<string, unknown> }[] = [];
  const relaying = (async () => {
    for await (const notice of userFacingTaskNotifications(tasks.notifications))
      woken.push(JSON.parse(notice));
  })();
  try {
    const handle = JSON.parse(
      (await call('implementChatto', { request: 'Fix', announcement: 'Starting' })).content[0]!
        .text!
    );
    await vi.waitFor(() => expect(woken.at(-1)?.type).toBe('task.completed'), { timeout: 10_000 });
    // Each stage reaches the supervisor as facts; it tells the user in its own words.
    expect(woken.map((notice) => notice.data?.milestone ?? notice.type)).toEqual([
      'validating',
      'published',
      'ci_failed',
      'ci_fix_pushed',
      'task.completed'
    ]);
    expect(woken[1]!.data).toEqual({
      milestone: 'published',
      prUrl: 'https://github.com/example/chatto/pull/7'
    });
    expect(woken[2]!.data).toMatchObject({
      attempt: 1,
      maxAttempts: 3,
      failedChecks: ['test-frontend-unit']
    });
    expect(createAgent).toHaveBeenCalledOnce();
    expect(prompts[1]).toContain('test-frontend-unit');
    expect(prompts[1]).toContain('FAIL src/a.spec.ts');
    const result = JSON.parse(tasks.get(handle.id).result!);
    expect(result).toMatchObject({ outcome: 'completed', ci: { status: 'passed', repairs: 1 } });
    expect(observed).toEqual([
      { headCommit: expect.any(String), stopOnFailure: true },
      { headCommit: result.commit, stopOnFailure: true }
    ]);
    expect(observed[0]!.headCommit).not.toBe(result.commit);
    expect(await remoteSubjects(f, result.branch)).toEqual([
      'fix: address CI failures',
      proposal.title,
      'fixture'
    ]);
    expect(approvedActions).toEqual([
      'prepare_pull_request',
      'publish_pull_request',
      'publish_pull_request_update'
    ]);
  } finally {
    await tasks.dispose();
    await relaying;
  }
});

test('unrelated CI failures are rerun when their runs finish, and new failures reach the worker first', async () => {
  const f = await fixture();
  const approvedActions: string[] = [];
  const job = (id: number, name: string) => ({
    name,
    link: `https://github.com/example/chatto/actions/runs/12/job/${id}`
  });
  const license = job(1, 'license-check');
  const e2e = job(2, 'test-e2e');
  const sequence = [
    { ...failedChecks, failures: [license] },
    { ...failedChecks, failed: 2, failures: [license, e2e] },
    { ...failedChecks, failed: 2, pending: 0, failures: [license, e2e] },
    passedChecks
  ];
  const observed: { known: string[]; initialDelayMs?: number }[] = [];
  const observeChecks = vi.fn(
    async (options: { knownFailures?: ReadonlySet<string>; initialDelayMs?: number }) => {
      observed.push({
        known: [...(options.knownFailures ?? [])],
        initialDelayMs: options.initialDelayMs
      });
      return sequence[observed.length - 1]!;
    }
  );
  const execute = vi.fn<ImplementationProcess>(async (command, args, options) =>
    command === 'gh' && (args[0] === 'api' || args[1] === 'rerun')
      ? 'flaky'
      : f.execute(command, args, options)
  );
  const prompts: string[] = [];
  const result = await createImplementation(f.settings, {
    execute,
    requestApproval: async (_ctx, action) => {
      approvedActions.push(action.action);
      return { decision: 'allow', reason: 'Within the delegated CI work.' };
    },
    observeChecks,
    rerunDelayMs: 5,
    createAgent: async (options: AgentOptions) => ({
      dispose: vi.fn(),
      async runOutcome(_ctx: unknown, prompt: string) {
        prompts.push(prompt);
        const call = await workerTools(options);
        const rerun = await call('rerunFailedChecks', {});
        if (prompts.length === 1) {
          expect(rerun.content[0]?.text).toContain('No CI failure');
          await call('apply_patch', { patch });
          await call('preparePullRequest', proposal);
        } else expect(rerun.content[0]?.text).toContain('Rerun requested');
        return { outcome: 'completed' as const, summary: 'Ready', usage: emptyTokenUsage() };
      }
    })
  })(createWorkflowContext(), { request: 'Fix' });
  expect(result).toMatchObject({ outcome: 'completed', ci: { status: 'passed', repairs: 2 } });
  // The second failure reaches the worker while the first one waits for its run to finish.
  expect(JSON.parse(prompts[1]!.split('\n')[1]!).failedChecks).toEqual(['license-check']);
  expect(JSON.parse(prompts[2]!.split('\n')[1]!).failedChecks).toEqual(['test-e2e']);
  expect(observed).toEqual([
    { known: [], initialDelayMs: 0 },
    { known: [license.link], initialDelayMs: 0 },
    { known: [license.link, e2e.link], initialDelayMs: 0 },
    { known: [], initialDelayMs: 5 }
  ]);
  expect(
    execute.mock.calls.filter(([command, args]) => command === 'gh' && args[1] === 'rerun')
  ).toEqual([
    ['gh', ['run', 'rerun', '12', '--failed', '--repo', 'example/chatto'], expect.anything()]
  ]);
  expect(await remoteSubjects(f, result.branch)).toEqual([proposal.title, 'fixture']);
  expect(approvedActions).toEqual([
    'prepare_pull_request',
    'publish_pull_request',
    'rerun_failed_checks'
  ]);
});

test.each(['publish_pull_request_update', 'rerun_failed_checks'] as const)(
  'owner denial prevents the CI effect: %s',
  async (deniedAction) => {
    const f = await fixture();
    let turn = 0;
    const actions: string[] = [];
    const execute = vi.fn<ImplementationProcess>(async (command, args, options) =>
      command === 'gh' && args[0] === 'api'
        ? 'Reference failure log.'
        : f.execute(command, args, options)
    );
    const result = await createImplementation(f.settings, {
      execute,
      observeChecks: async () => ({ ...failedChecks, pending: 0 }),
      requestApproval: async (_ctx, action) => {
        actions.push(action.action);
        return {
          decision: action.action === deniedAction ? 'deny' : 'allow',
          reason: 'Owner reviewed the action.'
        };
      },
      createAgent: worker(async (_options, call) => {
        if (++turn === 1) {
          await call('apply_patch', { patch });
          await call('preparePullRequest', proposal);
        } else if (deniedAction === 'publish_pull_request_update')
          await call('apply_patch', { patch: regressionPatch });
        else await call('rerunFailedChecks', {});
      })
    })(createWorkflowContext(), { request: 'Fix the value' });
    expect(result).toMatchObject({ outcome: 'completed', ci: { status: 'unfixed' } });
    expect(actions).toContain(deniedAction);
    expect(
      execute.mock.calls.filter(([command, args]) => command === 'git' && args.includes('push'))
    ).toHaveLength(1);
    expect(
      execute.mock.calls.some(([command, args]) => command === 'gh' && args[1] === 'rerun')
    ).toBe(false);
    expect(await remoteSubjects(f, result.branch)).toEqual([proposal.title, 'fixture']);
  }
);

test.each(['limit', 'stopped'])(
  'CI repair ends with the PR kept when the %s is reached',
  async (mode) => {
    const f = await fixture();
    let turns = 0;
    let jobs = 0;
    const result = await createImplementation(f.settings, {
      execute: async (command, args, options) =>
        command === 'gh' && (args[0] === 'api' || args[1] === 'rerun')
          ? ''
          : f.execute(command, args, options),
      // Each observation reports a new failed job, so every one goes to the worker.
      observeChecks: async () => ({
        ...failedChecks,
        failures: [
          { name: 'e2e', link: `https://github.com/example/chatto/actions/runs/12/job/${++jobs}` }
        ]
      }),
      rerunDelayMs: 0,
      createAgent: async (options: AgentOptions) => ({
        dispose: vi.fn(),
        async runOutcome() {
          const call = await workerTools(options);
          if (++turns === 1) {
            await call('apply_patch', { patch });
            await call('preparePullRequest', proposal);
          } else if (mode === 'limit') await call('rerunFailedChecks', {});
          else
            return {
              outcome: 'blocked' as const,
              summary: 'The failure needs a product decision',
              usage: emptyTokenUsage()
            };
          return { outcome: 'completed' as const, summary: 'Ready', usage: emptyTokenUsage() };
        }
      })
    })(createWorkflowContext(), { request: 'Fix' });
    expect(result.outcome).toBe('completed');
    expect(result.prUrl).toBe('https://github.com/example/chatto/pull/7');
    if (mode === 'limit') {
      expect(result).toMatchObject({ ci: { status: 'failed', repairs: 3 } });
      expect(turns).toBe(4);
    } else
      expect(result).toMatchObject({
        ci: {
          status: 'unfixed',
          repairs: 1,
          reason: expect.stringContaining('needs a product decision')
        }
      });
  }
);

test('a message that arrives while CI runs reaches the same worker, and its edits are pushed', async () => {
  const f = await fixture();
  const observing = Promise.withResolvers<void>();
  let observations = 0;
  const observeChecks = vi.fn(async (options: { signal: AbortSignal }) => {
    if (++observations === 1) {
      observing.resolve();
      await new Promise((_resolve, reject) =>
        options.signal.addEventListener('abort', () => reject(options.signal.reason), {
          once: true
        })
      );
    }
    return passedChecks;
  });
  const prompts: string[] = [];
  const emitted: unknown[] = [];
  const result = await createImplementation(f.settings, {
    execute: f.execute,
    observeChecks,
    createAgent: async (options: AgentOptions) => ({
      dispose: vi.fn(),
      async runOutcome(_ctx: unknown, prompt: string) {
        prompts.push(prompt);
        const call = await workerTools(options);
        if (prompts.length === 1) {
          await call('apply_patch', { patch });
          await call('preparePullRequest', proposal);
        } else await call('apply_patch', { patch: regressionPatch });
        return { outcome: 'completed' as const, summary: 'Ready', usage: emptyTokenUsage() };
      }
    })
  })(
    {
      ...createWorkflowContext(),
      emit: async (update: unknown) => void emitted.push(update),
      inbox: (async function* () {
        await observing.promise;
        yield 'Also add the regression file';
      })()
    },
    { request: 'Fix' }
  );
  expect(prompts[1]).toContain('Also add the regression file');
  expect(result).toMatchObject({ outcome: 'completed', ci: { status: 'passed', repairs: 0 } });
  expect(await remoteSubjects(f, result.branch)).toEqual([
    'chore: apply follow-up changes',
    proposal.title,
    'fixture'
  ]);
  // The worker's answer to the message reaches the parent, which tells the user.
  expect(emitted).toContainEqual({
    type: 'notice',
    text: 'The worker handled the forwarded messages and answered: Ready',
    data: { milestone: 'messages_handled' }
  });
});

test('an unexpected worker error produces one safe stopped result for the owner', async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx, { notifyActivity: false });
  const extension = implementationExtension(ctx, f.settings, async () => {}, tasks, {
    execute: f.execute,
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
    expect(JSON.stringify(tasks.get(handle.id))).not.toContain('private provider detail');
    expect(JSON.parse(tasks.get(handle.id).result!)).toMatchObject({
      outcome: 'blocked',
      summary:
        'The implementation stopped during editing because of an unexpected error. The work so far is kept.'
    });
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
  vi.stubEnv('CHATTO_SOURCE_REF', ' ');
  expect(implementationSettings()?.baseBranch).toBe('main');
  vi.stubEnv('CHATTO_SOURCE_REF', 'origin/develop');
  expect(implementationSettings()?.baseBranch).toBe('develop');
  vi.stubEnv('CHATTO_SOURCE_REF', 'v1.0^{commit}');
  expect(() => implementationSettings()).toThrow(ConfigurationError);
  vi.stubEnv('CHATTO_SOURCE_REF', '');
  vi.stubEnv('CHATTO_IMPLEMENTATION_REPOSITORY', 'https://github.com/example/chatto');
  expect(() => implementationSettings()).toThrow(
    'CHATTO_IMPLEMENTATION_REPOSITORY must be owner/repo'
  );
  vi.stubEnv('CHATTO_IMPLEMENTATION_REPOSITORY', 'example/chatto');
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
      // The host passes the issue that it read to the worker.
      expect(JSON.parse(prompt).issue).toMatchObject({ number: 12, title: 'Wrong value' });
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
    plans: new Map([['investigation', plan]]),
    fetchIssue: async (number) => ({
      repository: 'example/chatto',
      number,
      title: 'Wrong value',
      body: 'The value is wrong.',
      url: `https://github.com/example/chatto/issues/${number}`
    })
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
      issueNumber: 12,
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
    // Milestone notices come first; completion is last.
    let notification = JSON.parse((await notices.next()).value!);
    while (notification.type === 'task.notice')
      notification = JSON.parse((await notices.next()).value!);
    expect(notification.type).toBe('task.completed');
    expect(JSON.parse(notification.task.result)).toMatchObject({
      outcome: 'completed',
      prUrl: 'https://github.com/example/chatto/pull/7'
    });
    expect(f.body).toMatch(/\n\nCloses #12\.$/);
    expect(announce).toHaveBeenCalledOnce();
  } finally {
    delivered.resolve();
    finish.resolve();
    await tasks.dispose();
  }
});

test('a parallel call is refused while the first reads its issue; a failed read frees the attempt', async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx);
  const read = Promise.withResolvers<never>();
  const fetchIssue = vi.fn(() => read.promise);
  const onBlocked = vi.fn(async () => {});
  let version = 1;
  const call = await workerTools({
    cwd: f.settings.directory,
    model: 'test/model',
    extensions: [
      implementationExtension(ctx, f.settings, async () => {}, tasks, {
        execute: f.execute,
        onBlocked,
        fetchIssue,
        requestVersion: () => version
      })
    ]
  });
  const input = { request: 'Fix', issueNumber: 12, announcement: 'Starting' };
  try {
    const first = call('implementChatto', input);
    const second = JSON.parse((await call('implementChatto', input)).content[0]!.text!);
    expect(second).toMatchObject({ outcome: 'blocked' });
    // A newer request does not start a second implementation while the first one starts.
    version = 2;
    const third = JSON.parse((await call('implementChatto', input)).content[0]!.text!);
    expect(third).toMatchObject({ outcome: 'blocked' });
    expect(onBlocked).toHaveBeenCalledTimes(2);
    version = 1;
    read.reject(new Error('Issue not found'));
    await expect(first).rejects.toThrow('Issue not found');
    // The failed read did not start work, so the same request can try again.
    await expect(call('implementChatto', input)).rejects.toThrow('Issue not found');
    expect(fetchIssue).toHaveBeenCalledTimes(2);
  } finally {
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
        scripts: {
          check: 'node --check regression.cjs',
          lint: 'node --check regression.cjs',
          test: 'node --test regression.cjs'
        }
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

test('failed patch context stays inside the worktree', async () => {
  const root = await mkdtemp(join(tmpdir(), 'chattobot-hunk-'));
  folders.push(root);
  await mkdir(join(root, 'worktree'));
  await writeFile(join(root, 'outside.txt'), 'secret\n');
  await writeFile(join(root, 'worktree', 'inside.txt'), 'a\nb\n');
  const worktree = join(root, 'worktree');
  expect(await failedHunkLines(worktree, 'error: patch failed: ../outside.txt:1')).toBe('');
  expect(await failedHunkLines(worktree, 'error: patch failed: missing.txt:1')).toBe('');
  expect(await failedHunkLines(worktree, 'error: no hunk')).toBe('');
  expect(await failedHunkLines(worktree, 'error: patch failed: inside.txt:2')).toContain(
    'Current lines 1-3 of inside.txt:\n1: a\n2: b'
  );
});
