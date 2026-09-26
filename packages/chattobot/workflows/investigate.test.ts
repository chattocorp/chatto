import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, readFile, writeFile, rm, readdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import { createWorkflowContext, emptyTokenUsage } from 'runling';
import type { AgentExtensionAPI, AgentOptions } from 'runling/agents';
import { createAgentTasks } from 'runling/agents';
import {
  createInvestigation,
  investigationExtension,
  investigationSettings
} from './investigate.ts';
import type { Finding } from './evidence.ts';
import { responsePolicy } from './response-policy.ts';
import type { InvestigationPlans } from './plan.ts';

const mocks = vi.hoisted(() => ({ agent: vi.fn() }));
vi.mock('runling/agents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling/agents')>()),
  agent: mocks.agent
}));

const exec = promisify(execFile);
const folders: string[] = [];
afterEach(async () => {
  vi.unstubAllEnvs();
  await Promise.all(
    folders.splice(0).map((folder) => rm(folder, { recursive: true, force: true }))
  );
});
async function fixture() {
  const folder = await mkdtemp(join(tmpdir(), 'chattobot-investigate-'));
  folders.push(folder);
  const directory = join(folder, 'repo');
  await exec('git', ['init', directory]);
  await writeFile(join(directory, 'example.txt'), 'original\n');
  await exec('git', ['add', '.'], { cwd: directory });
  await exec(
    'git',
    [
      '-c',
      'user.name=Test',
      '-c',
      'user.email=test@example.invalid',
      '-c',
      'commit.gpgsign=false',
      'commit',
      '-m',
      'fixture'
    ],
    { cwd: directory }
  );
  return { directory, artifactsDirectory: join(folder, 'artifacts') };
}

test('parallel investigations use detached worktrees and expose only read-only tools', async () => {
  const settings = await fixture();
  await writeFile(join(settings.directory, 'example.txt'), 'host dirty change\n');
  const paths: string[] = [];
  const dispose = vi.fn();
  const investigate = createInvestigation(settings, async (options) => {
    paths.push(options.cwd);
    expect(await readFile(join(options.cwd, 'example.txt'), 'utf8')).toBe('original\n');
    expect(options.tools).toEqual([
      'read',
      'grep',
      'find',
      'ls',
      'recordFinding',
      'prepareImplementationPlan'
    ]);
    expect(options.resources).toMatchObject({
      extensions: false,
      skills: false,
      promptTemplates: false
    });
    expect(options.instructions?.join('\n')).toContain('You cannot edit files.');
    let record!: (id: string, finding: Finding) => Promise<unknown>;
    const extension = options.extensions![0]!;
    const factory = typeof extension === 'function' ? extension : extension.factory;
    await factory({
      registerTool(tool: { execute: typeof record }) {
        record = tool.execute;
      }
    } as unknown as AgentExtensionAPI);
    return {
      dispose,
      async runOutcome() {
        await record('evidence', {
          claim: 'The file contains the original value.',
          kind: 'observation',
          evidence: [{ path: 'example.txt', startLine: 1, endLine: 1, quote: 'original' }]
        });
        return {
          outcome: 'completed',
          summary: 'Inspected',
          details: 'Evidence: example.txt:1. Tests not run: read-only investigation.',
          usage: emptyTokenUsage()
        };
      }
    };
  });
  const results = await Promise.all(
    ['first', 'second'].map((question) => investigate(createWorkflowContext(), { question }))
  );
  expect(new Set(paths).size).toBe(2);
  expect(dispose).toHaveBeenCalledTimes(2);
  for (const result of results) {
    expect(result.outcome).toBe('completed');
    expect(result.findings).toHaveLength(1);
    expect(result.validation).toEqual({
      citationsChecked: true,
      reproduced: false,
      testsRun: false,
      changesApplied: false
    });
    expect(result.baseCommit).toMatch(/^[0-9a-f]{40,64}$/);
    expect(await readFile(result.patch, 'utf8')).toBe('');
    expect(await readFile(join(result.worktree, 'example.txt'), 'utf8')).toBe('original\n');
    expect(
      (
        await exec('git', ['rev-parse', '--abbrev-ref', 'HEAD'], { cwd: result.worktree })
      ).stdout.trim()
    ).toBe('HEAD');
  }
  expect(await readFile(join(settings.directory, 'example.txt'), 'utf8')).toBe(
    'host dirty change\n'
  );
});

test.each(['cancel', 'timeout', 'failure'])(
  'disposes the agent and retains evidence on %s',
  async (mode) => {
    const settings = await fixture();
    const controller = new AbortController();
    const dispose = vi.fn();
    const investigate = createInvestigation(
      { ...settings, timeoutMs: mode === 'timeout' ? 200 : 10_000 },
      async () => ({
        dispose,
        async runOutcome(ctx) {
          if (mode === 'failure') throw new Error('Model failed');
          if (mode === 'cancel') controller.abort(new Error('Cancelled'));
          ctx.signal.throwIfAborted();
          return new Promise((_resolve, reject) =>
            ctx.signal.addEventListener('abort', () => reject(ctx.signal.reason), { once: true })
          );
        }
      })
    );
    await expect(
      investigate(
        { ...createWorkflowContext(), signal: controller.signal },
        { question: 'Investigate' }
      )
    ).rejects.toThrow();
    expect(dispose).toHaveBeenCalledOnce();
    const [folder] = await readdir(settings.artifactsDirectory);
    expect(
      await readFile(join(settings.artifactsDirectory, folder!, 'changes.patch'), 'utf8')
    ).toBe('');
  }
);

test('invalid refs fail before creating an agent', async () => {
  const settings = await fixture();
  const factory = vi.fn();
  await expect(
    createInvestigation({ ...settings, baseRef: '--help' }, factory)(createWorkflowContext(), {
      question: 'test'
    })
  ).rejects.toThrow();
  expect(factory).not.toHaveBeenCalled();
});

test('source access is opt-in and captures host settings', () => {
  vi.stubEnv('CHATTO_SOURCE_DIRECTORY', '');
  expect(investigationSettings()).toBeUndefined();
  vi.stubEnv('CHATTO_SOURCE_DIRECTORY', '/configured/repo');
  vi.stubEnv('CHATTO_SOURCE_REF', 'origin/main');
  const settings = investigationSettings();
  vi.stubEnv('CHATTO_SOURCE_DIRECTORY', '/different/repo');
  expect(settings).toMatchObject({ directory: '/configured/repo', baseRef: 'origin/main' });
});

test.each(['direct', 'tool'])(
  '%s investigation defaults to assessment without requiring a plan',
  async (entry) => {
    const settings = await fixture();
    const ctx = createWorkflowContext();
    const tasks = createAgentTasks(ctx, { notifyActivity: false });
    const prompts: string[] = [];
    const factory = async (options: AgentOptions) => {
      let record!: (id: string, finding: Finding) => Promise<unknown>;
      const extension = options.extensions![0]!;
      const install = typeof extension === 'function' ? extension : extension.factory;
      await install({
        registerTool(tool: { execute: typeof record }) {
          record = tool.execute;
        }
      } as unknown as AgentExtensionAPI);
      return {
        dispose: () => {},
        async runOutcome(_ctx: unknown, prompt: string) {
          prompts.push(prompt);
          await record('finding', {
            claim: 'Contains original',
            kind: 'observation',
            evidence: [{ path: 'example.txt', startLine: 1, endLine: 1 }]
          });
          return { outcome: 'completed' as const, summary: 'Checked', usage: emptyTokenUsage() };
        }
      };
    };
    try {
      let result;
      if (entry === 'direct')
        result = await createInvestigation(settings, factory)(ctx, {
          question: 'Read the fixture'
        });
      else {
        mocks.agent.mockImplementation(factory);
        let call!: (id: string, input: unknown) => Promise<{ content: { text: string }[] }>;
        const extension = investigationExtension(ctx, settings, async () => {}, tasks);
        const install = typeof extension === 'function' ? extension : extension.factory;
        await install({
          registerTool(tool: { execute: typeof call }) {
            call = tool.execute;
          }
        } as unknown as AgentExtensionAPI);
        const handle = JSON.parse(
          (await call('call', { question: 'Read the fixture', announcement: 'Checking' }))
            .content[0]!.text
        );
        await vi.waitFor(() => expect(tasks.get(handle.id).status).toBe('completed'));
        result = JSON.parse(tasks.get(handle.id).result!);
      }
      expect(result).toMatchObject({ outcome: 'completed' });
      expect(result.plan).toBeUndefined();
      expect(prompts).toHaveLength(1);
      expect(JSON.parse(prompts[0]!)).toMatchObject({ purpose: 'assessment' });
    } finally {
      await tasks.dispose();
    }
  }
);

test.each([true, false])(
  'implementation investigations retain a typed plan only after successful delivery: %s',
  async (withPlan) => {
    const settings = await fixture();
    const ctx = createWorkflowContext();
    const tasks = createAgentTasks(ctx, { notifyActivity: false });
    const plans: InvestigationPlans = new Map();
    const plan = {
      goal: 'Fix the fixture',
      steps: [{ files: ['example.txt'], change: 'Replace original with fixed' }],
      acceptanceCriteria: ['Value is fixed'],
      checks: ['Read the value'],
      openQuestions: []
    };
    mocks.agent.mockImplementation(async (options: AgentOptions) => {
      const tools = new Map<string, (id: string, input: unknown) => Promise<unknown>>();
      for (const extension of options.extensions ?? []) {
        const factory = typeof extension === 'function' ? extension : extension.factory;
        await factory({
          registerTool(tool: {
            name: string;
            execute: (id: string, input: unknown) => Promise<unknown>;
          }) {
            tools.set(tool.name, tool.execute);
          }
        } as unknown as AgentExtensionAPI);
      }
      return {
        dispose: () => {},
        async runOutcome() {
          await tools.get('recordFinding')!('finding', {
            claim: 'Contains original',
            kind: 'observation',
            evidence: [{ path: 'example.txt', startLine: 1, endLine: 1 }]
          });
          if (withPlan) await tools.get('prepareImplementationPlan')!('plan', plan);
          return { outcome: 'completed', summary: 'Done', usage: emptyTokenUsage() };
        }
      };
    });
    let call!: (id: string, input: unknown) => Promise<{ content: { text: string }[] }>;
    const extension = investigationExtension(ctx, settings, async () => {}, tasks, plans);
    const factory = typeof extension === 'function' ? extension : extension.factory;
    await factory({
      registerTool(tool: { execute: typeof call }) {
        call = tool.execute;
      }
    } as unknown as AgentExtensionAPI);
    try {
      const handle = JSON.parse(
        (
          await call('call', {
            question: 'Plan a fix',
            purpose: 'implementation',
            announcement: 'Investigating'
          })
        ).content[0]!.text
      );
      await vi.waitFor(() => expect(tasks.get(handle.id).status).toBe('completed'));
      const result = JSON.parse(tasks.get(handle.id).result!);
      expect(result.outcome).toBe(withPlan ? 'completed' : 'blocked');
      if (withPlan) {
        expect(plans.get(handle.id)).toEqual({ ...plan, baseCommit: result.baseCommit });
        expect(result.findings[0].evidence[0].quote).toBe('original');
      } else expect(plans.size).toBe(0);
    } finally {
      await tasks.dispose();
    }
  }
);

test('repairs the loud-waves handoff in the same worker before the owner receives completion', async () => {
  const settings = await fixture();
  const tasks = createAgentTasks(createWorkflowContext());
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const prompts: string[] = [];
  const dispose = vi.fn();
  const createAgent = vi.fn(async (options: AgentOptions) => {
    let record!: (id: string, finding: Finding) => Promise<unknown>;
    const extension = options.extensions![0]!;
    const factory = typeof extension === 'function' ? extension : extension.factory;
    await factory({
      registerTool(tool: { execute: typeof record }) {
        record = tool.execute;
      }
    } as unknown as AgentExtensionAPI);
    return {
      dispose,
      async runOutcome(_ctx: unknown, prompt: string) {
        prompts.push(prompt);
        expect(await readFile(join(options.cwd, 'example.txt'), 'utf8')).toBe('original\n');
        if (prompts.length === 2)
          await record('repaired', {
            claim: 'The fixture contains the original value.',
            kind: 'observation',
            evidence: [{ path: 'example.txt', startLine: 1, endLine: 1, quote: 'original' }]
          });
        return {
          outcome: 'completed' as const,
          summary: 'I found the answer',
          details: 'Unvalidated prose',
          usage: emptyTokenUsage()
        };
      }
    };
  });
  try {
    const investigate = createInvestigation(settings, createAgent);
    tasks.start(
      'Investigate',
      async (ctx) =>
        JSON.stringify(await investigate(ctx, { question: 'What does the fixture contain?' })),
      undefined
    );
    const notice = JSON.parse((await reader.next()).value!);
    expect(notice.type).toBe('task.completed');
    const result = JSON.parse(notice.task.result);
    expect(result).toMatchObject({
      outcome: 'completed',
      findings: [{ evidence: [{ path: 'example.txt', startLine: 1 }] }]
    });
    expect(result.failureReason).toBeUndefined();
    expect(notice.task.result).not.toContain('Unvalidated prose');
    expect(prompts).toHaveLength(2);
    expect(prompts[1]).toContain('No recordFinding call was accepted');
    expect(createAgent).toHaveBeenCalledOnce();
    expect(dispose).toHaveBeenCalledOnce();
    expect(await readdir(settings.artifactsDirectory)).toHaveLength(1);
  } finally {
    await tasks.dispose();
  }
});

test.each([
  { outcome: 'completed' as const, reason: undefined, expected: 'missing_evidence', calls: 2 },
  {
    outcome: 'failed' as const,
    reason: 'missing_outcome' as const,
    expected: 'missing_outcome',
    calls: 2
  },
  {
    outcome: 'failed' as const,
    reason: 'provider_error' as const,
    expected: 'provider_error',
    calls: 1
  },
  { outcome: 'blocked' as const, reason: undefined, expected: 'worker_blocked', calls: 1 }
])(
  'reports $expected without an unbounded repair loop',
  async ({ outcome, reason, expected, calls }) => {
    const settings = await fixture();
    const runOutcome = vi.fn(async () => ({
      outcome,
      failureReason: reason,
      summary: 'Worker result',
      usage: emptyTokenUsage()
    }));
    const result = await createInvestigation(settings, async () => ({
      dispose: () => {},
      runOutcome
    }))(createWorkflowContext(), { question: 'Inspect' });
    expect(result.failureReason).toBe(expected);
    expect(result.outcome).not.toBe('completed');
    expect(runOutcome).toHaveBeenCalledTimes(calls);
  }
);

test('cancellation during report repair disposes the same worker', async () => {
  const settings = await fixture();
  const controller = new AbortController();
  const dispose = vi.fn();
  let turns = 0;
  await expect(
    createInvestigation(settings, async () => ({
      dispose,
      async runOutcome() {
        if (++turns === 2) controller.abort();
        return { outcome: 'completed', summary: 'No evidence', usage: emptyTokenUsage() };
      }
    }))({ ...createWorkflowContext(), signal: controller.signal }, { question: 'Inspect' })
  ).rejects.toThrow();
  expect(turns).toBe(2);
  expect(dispose).toHaveBeenCalledOnce();
});

// Opt-in model evaluation of real file tools, citations, completion, and owner reply.
test.skipIf(!process.env.CHATTO_EVAL_MODEL)(
  'live worker hands checked evidence to the owner',
  async () => {
    const settings = await fixture();
    const real = await vi.importActual<typeof import('runling/agents')>('runling/agents');
    const model = process.env.CHATTO_EVAL_MODEL!;
    const ctx = createWorkflowContext();
    const result = await createInvestigation({ ...settings, model, timeoutMs: 90_000 }, (options) =>
      real.agent({
        ...options,
        onEvent: (event) => {
          // This opt-in fixture contains only synthetic data. Show tool validation
          // errors so a failed live evaluation identifies the broken handoff.
          if (
            event.type === 'tool_execution_end' &&
            event.toolName === 'recordFinding' &&
            event.isError
          )
            console.info('Synthetic citation validation:', JSON.stringify(event.result));
        }
      })
    )(ctx, {
      question:
        'This synthetic test repository contains only example.txt. There are no AGENTS.md files. What exact value does example.txt contain?'
    });
    expect(result.outcome).toBe('completed');
    expect(
      result.findings.some((finding) =>
        finding.evidence.some(
          (citation) => citation.path === 'example.txt' && citation.quote?.trim() === 'original'
        )
      )
    ).toBe(true);
    const owner = await real.agent({
      cwd: settings.directory,
      model,
      output: 'text',
      tools: [],
      resources: {
        extensions: false,
        skills: false,
        contextFiles: false,
        promptTemplates: false,
        themes: false
      },
      instructions: [
        "You are ChattoBot. Briefly answer the user's question from the completed investigation.",
        ...responsePolicy
      ]
    });
    let reply = '';
    try {
      await owner.runOutcome(
        ctx,
        JSON.stringify({
          origin: 'notification',
          recentUserMessages: ['What does example.txt contain?'],
          backgroundTasks: [{ status: 'completed', result }],
          notification: { type: 'task.completed' }
        }),
        {
          signal: AbortSignal.timeout(60_000),
          onText: (text) => {
            reply += text;
          }
        }
      );
      expect(reply).toContain('original');
      expect(reply).toMatch(/example\.txt/);
      expect(reply).toMatch(/tests were not run/i);
      expect(reply).not.toMatch(/still (?:working|investigating)|try again/i);
    } finally {
      owner.dispose();
    }
  },
  160_000
);

test('background investigations retain findings without chat notifications and accept supervisor steering', async () => {
  const settings = await fixture();
  const tasks = createAgentTasks(createWorkflowContext(), { progressIntervalMs: 0 });
  const reader = tasks.notifications[Symbol.asyncIterator]();
  const started = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<void>();
  const steer = vi.fn(async () => true);
  let workerOptions: AgentOptions | undefined;
  const investigate = createInvestigation(settings, async (options) => {
    workerOptions = options;
    let record!: (id: string, finding: Finding) => Promise<unknown>;
    const extension = options.extensions![0]!;
    const factory = typeof extension === 'function' ? extension : extension.factory;
    await factory({
      registerTool(tool: { execute: typeof record }) {
        record = tool.execute;
      }
    } as unknown as AgentExtensionAPI);
    return {
      steer,
      dispose: () => {},
      async runOutcome(_ctx, _prompt, options) {
        options?.onText?.('The two pages use different layout components.');
        await record('finding', {
          claim: 'The fixture contains the original value.',
          kind: 'observation',
          evidence: [{ path: 'example.txt', startLine: 1, endLine: 1, quote: 'original' }]
        });
        started.resolve();
        await finish.promise;
        return { outcome: 'completed', summary: 'Checked', usage: emptyTokenUsage() };
      }
    };
  });
  const handle = tasks.start(
    'Investigate',
    async (ctx, question: string) => JSON.stringify(await investigate(ctx, { question })),
    'Compare pages'
  );
  try {
    await started.promise;
    await vi.waitFor(() =>
      expect(
        tasks
          .get(handle.id)
          .output.some((item) => item.text.includes('The fixture contains the original value.'))
      ).toBe(true)
    );
    expect(tasks.get(handle.id).progress).toBeUndefined();
    workerOptions!.onStatus!({ type: 'retrying', attempt: 1, maxAttempts: 3, delayMs: 2000 });
    expect(JSON.parse((await reader.next()).value!)).toMatchObject({
      type: 'task.retrying',
      task: {
        provider: { type: 'retrying', attempt: 1 }
      }
    });
    workerOptions!.onActivity!({ type: 'tool', operation: 'edit', phase: 'failed', failures: 2 });
    expect(JSON.parse((await reader.next()).value!)).toMatchObject({
      type: 'task.tool_failed',
      task: {
        activity: { operation: 'edit', phase: 'failed', failures: 2 },
        lastToolFailure: { operation: 'edit', phase: 'failed' }
      }
    });
    expect(
      tasks
        .get(handle.id)
        .output.some((item) => item.text.includes('The fixture contains the original value.'))
    ).toBe(true);
    await tasks.send(handle.id, 'Only check private channels');
    await vi.waitFor(() => expect(steer).toHaveBeenCalledWith('Only check private channels'));
    finish.resolve();
    expect(JSON.parse((await reader.next()).value!).type).toBe('task.completed');
  } finally {
    finish.resolve();
    await tasks.dispose();
  }
});

test('the registered tool runs a child workflow and returns its evidence', async () => {
  const settings = await fixture();
  const dispose = vi.fn();
  mocks.agent.mockResolvedValue({
    dispose,
    runOutcome: async () => ({
      outcome: 'completed',
      summary: 'Checked the report',
      details: 'Evidence: example.txt:1',
      usage: emptyTokenUsage()
    })
  });
  let execute!: (
    id: string,
    input: { question: string; announcement: string },
    signal?: AbortSignal
  ) => Promise<{ content: Array<{ text: string }> }>;
  let release!: () => void;
  const delivered = new Promise<void>((resolve) => {
    release = resolve;
  });
  const announce = vi.fn(async () => delivered);
  mocks.agent.mockClear();
  const tasks = createAgentTasks(createWorkflowContext());
  const notifications = tasks.notifications[Symbol.asyncIterator]();
  const extension = investigationExtension(
    {
      ...createWorkflowContext(),
      inbox: (async function* () {
        yield 'unused follow-up';
      })(),
      emit: async (_text: string) => {}
    },
    settings,
    announce,
    tasks
  );
  const factory = typeof extension === 'function' ? extension : extension.factory;
  await factory({
    registerTool(tool: { name: string; execute: typeof execute }) {
      expect(tool.name).toBe('investigateChatto');
      execute = tool.execute;
    }
  } as unknown as AgentExtensionAPI);
  const pending = execute('call', {
    question: 'Is this a bug?',
    announcement: "I'll trace the notification code."
  });
  await vi.waitFor(() => expect(announce).toHaveBeenCalledOnce());
  expect(announce).toHaveBeenCalledWith(
    "I'll trace the notification code.",
    expect.any(AbortSignal)
  );
  expect(mocks.agent).not.toHaveBeenCalled();
  await expect(readdir(settings.artifactsDirectory)).rejects.toMatchObject({ code: 'ENOENT' });
  release();
  const result = await pending;
  const handle = JSON.parse(result.content[0]!.text);
  expect(handle).toMatchObject({ status: 'running' });
  const notification = JSON.parse((await notifications.next()).value!);
  expect(notification.type).toBe('task.completed');
  expect(JSON.parse(tasks.get(handle.id).result!)).toMatchObject({
    outcome: 'blocked',
    findings: [],
    validation: { citationsChecked: false, testsRun: false, changesApplied: false }
  });
  expect(tasks.get(handle.id).result).not.toContain('Evidence: example.txt:1');
  expect(dispose).toHaveBeenCalledOnce();
  const aborted = new AbortController();
  aborted.abort(new Error('cancel tool'));
  await expect(
    execute('cancelled', { question: 'test', announcement: 'Checking.' }, aborted.signal)
  ).rejects.toThrow('cancel tool');
  announce.mockRejectedValueOnce(new Error('Posting failed'));
  await expect(execute('failed', { question: 'test', announcement: 'Checking.' })).rejects.toThrow(
    'Posting failed'
  );
  await expect(execute('blank', { question: 'test', announcement: '   ' })).rejects.toThrow(
    'announcement is required'
  );
  expect(mocks.agent).toHaveBeenCalledOnce();
  await tasks.dispose();
});
