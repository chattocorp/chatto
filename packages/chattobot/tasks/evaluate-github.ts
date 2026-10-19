/** Opt-in real-model evaluations. GitHub and Chatto are in-memory fixtures; only the model
 * provider is contacted. Run with `pnpm eval:github`, optionally followed by a case name. */
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { createAgentSession, emptyTokenUsage, log, task, Type, type TaskContext } from 'runling';
import { createConversation } from './conversation.ts';
import { createGitHubWorker, GitHubWorkInput } from './github.ts';
import { ConfigurationError, setting, thinkingSetting } from '../settings.ts';
import { FakeGitHub } from '../evaluations/fake-github.ts';
import { classifyGh } from '../github/policy.ts';
import { supervisorCases, workerCases, type SupervisorCase } from '../evaluations/github-cases.ts';
import { languageReport } from '../evaluations/language-report.ts';
import { commandCases } from '../evaluations/github-command-cases.ts';
import { createGitHubCommandClassifier } from './classify-github-command.ts';

interface CaseResult {
  name: string;
  passed: boolean;
  durationMs: number;
  error?: string;
  /** Synthetic commands and responses, stored in the private report, not operational logs. */
  evidence: unknown;
}

/** Run a real supervisor against fake Chatto. An initial synthetic turn lets the inbox collect
 * all case messages into one turn, including mixed-author tests. The evaluated turn is real. */
async function supervisor(
  ctx: TaskContext<unknown, unknown>,
  model: string,
  scenario: SupervisorCase
) {
  const github = scenario.fixture?.() ?? new FakeGitHub();
  const delegated: Parameters<ReturnType<typeof createGitHubWorker>>[1][] = [];
  let directWrites = 0;
  const attempts: { requestId?: unknown; task?: unknown; evidence?: unknown }[] = [];
  const investigationAttempts: { requestId?: unknown }[] = [];
  const investigations: string[] = [];
  const posts: string[] = [];
  const handled: boolean[] = [];
  const warmed = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const conversation = createConversation(
    {
      model,
      thinkingLevel: thinkingSetting('CHATTO_AGENT_THINKING', 'medium'),
      maintainers: new Set(['maintainer']),
      maintainerLogin: scenario.maintainerLogin,
      github: {
        ...github.settings(),
        run: (args, options) => {
          if (classifyGh(args).access === 'write') directWrites++;
          return github.run(args, options);
        }
      },
      investigation: {
        directory: '/evaluation/fixture',
        investigator: () =>
          task(
            {
              name: 'Fixture investigation',
              input: Type.Any(),
              result: Type.Any(),
              output: Type.String()
            },
            async (_ctx, input) => {
              investigations.push(input.question);
              return {
                session: input.id ?? input.session,
                answer: 'Fixture investigation found no confirmed cause.',
                citations: []
              };
            }
          ) as never
      }
    },
    {
      post: async (_destination, body) => {
        posts.push(body);
        return `post-${posts.length}`;
      },
      typing: async () => {},
      acknowledge: async () => {},
      setHandled: async (thread, value) => {
        assert.deepEqual(thread, { roomId: 'fixture-room', threadRootId: 'fixture-thread' });
        if (value && scenario.expectedRequest)
          assert(
            posts.some((body) => github.issues.some((issue) => body.includes(issue.url))),
            'Share the filing result before marking handled'
          );
        handled.push(value);
      },
      readThread: async () => ({ messages: scenario.thread ?? [], olderOmitted: false }),
      readAttachment: async () => {
        throw new Error('No evaluation attachments');
      }
    },
    async (options) => {
      const bot = await createAgentSession({
        ...options,
        // Other production tools cannot contact their real services during this evaluation.
        tools: [
          'gh',
          'githubWork',
          'investigate',
          'readThread',
          ...(scenario.automaticIntake ? ['setHandled'] : [])
        ],
        onEvent(event) {
          if (event.type === 'tool_execution_start' && event.toolName === 'githubWork')
            attempts.push(
              event.args as { requestId?: unknown; task?: unknown; evidence?: unknown }
            );
          if (event.type === 'tool_execution_start' && event.toolName === 'investigate')
            investigationAttempts.push(event.args as { requestId?: unknown });
        }
      });
      const run = bot.runOutcome.bind(bot);
      let first = true;
      bot.runOutcome = async (...args) => {
        if (!first) return run(...args);
        first = false;
        warmed.resolve();
        await release.promise;
        return { outcome: 'completed', summary: 'Ready.', usage: emptyTokenUsage() };
      };
      return bot;
    },
    (settings) => {
      const worker = scenario.realWorker
        ? createGitHubWorker({
            ...settings,
            ...github.settings()
          })
        : undefined;
      return task(
        {
          name: 'Fixture GitHub worker',
          input: GitHubWorkInput,
          result: Type.String()
        },
        async (workerContext, input) => {
          delegated.push(input);
          if (worker) return worker(workerContext, input);
          if (scenario.workerResult !== undefined) return scenario.workerResult;
          github.issues.push({
            number: 101,
            title: 'Fixture issue',
            body: input.task,
            labels: [],
            state: 'OPEN',
            url: 'https://github.com/evaluation/fixture/issues/101'
          });
          return 'Created https://github.com/evaluation/fixture/issues/101';
        }
      );
    }
  );
  const location = { roomId: 'fixture-room', threadRootId: 'fixture-thread' };
  await using invocation = ctx.start(conversation, {
    ...location,
    id: 'warmup',
    authorId: 'member',
    body: 'Hello',
    ...(scenario.automaticIntake ? { trigger: 'intake' as const } : {})
  });
  // If agent initialization fails, do not leave the evaluation waiting for a warmup forever.
  await Promise.race([
    warmed.promise,
    invocation.result.then(() => {
      throw new Error('Conversation ended before evaluation');
    })
  ]);
  for (const message of scenario.messages) await invocation.steer({ ...location, ...message });
  // Let the conversation's inbox reader queue the complete batch before releasing its turn.
  await new Promise<void>((done) => setImmediate(done));
  invocation.closeSteering();
  release.resolve();
  await invocation.result;
  return {
    delegated,
    directWrites,
    attempts,
    investigations,
    investigationAttempts,
    posts: posts.slice(1),
    handled,
    commands: github.commands
  };
}

export default task(
  {
    name: 'Evaluate GitHub behavior',
    input: Type.String(),
    result: Type.Object({ passed: Type.Integer(), total: Type.Integer(), report: Type.String() })
  },
  async (ctx, filter) => {
    const model = setting('CHATTO_AGENT_MODEL');
    if (!model) throw new ConfigurationError('Set CHATTO_AGENT_MODEL for evaluations');
    const selected = (name: string) =>
      !filter || filter === 'all' || name === filter || name.startsWith(`${filter}-`);
    const cases = [
      ...workerCases,
      ...supervisorCases,
      ...commandCases,
      { name: 'language-report-withdrawal' }
    ];
    if (!cases.some((scenario) => selected(scenario.name)))
      throw new Error(
        `Unknown evaluation case. Choose: ${cases.map((scenario) => scenario.name).join(', ')}`
      );
    const directory = resolve('.runling/evaluations', `github-${crypto.randomUUID()}`);
    await mkdir(directory, { recursive: true, mode: 0o700 });
    const results: CaseResult[] = [];

    async function evaluate(
      name: string,
      work: (
        context: TaskContext<unknown, unknown>,
        evidence: (value: unknown) => void
      ) => Promise<void>
    ) {
      const started = performance.now();
      let evidence: unknown;
      let error: string | undefined;
      try {
        await work(
          { ...ctx, signal: AbortSignal.any([ctx.signal, AbortSignal.timeout(120_000)]) },
          (value) => {
            evidence = value;
          }
        );
      } catch (cause) {
        ctx.signal.throwIfAborted();
        error = cause instanceof Error ? cause.message : String(cause);
      }
      results.push({
        name,
        passed: error === undefined,
        durationMs: Math.round(performance.now() - started),
        error,
        evidence
      });
      // Detailed failure evidence stays in the private report. Logs use fixed case names only.
      if (error) log.error(`FAIL ${name}`);
      else log.success(`PASS ${name}`);
      await writeFile(
        resolve(directory, 'report.json'),
        JSON.stringify({ model, results }, null, 2),
        { mode: 0o600 }
      );
    }

    for (const scenario of commandCases.filter((item) => selected(item.name))) {
      await evaluate(scenario.name, async (context, evidence) => {
        const decision = await createGitHubCommandClassifier(model)(context, {
          task: scenario.task,
          repository: 'evaluation/fixture',
          args: scenario.args
        });
        evidence(decision);
        assert.equal(decision.allow, scenario.allow, 'Incorrect command scope decision');
      });
    }
    for (const scenario of workerCases.filter((item) => selected(item.name))) {
      await evaluate(scenario.name, async (context, evidence) => {
        const github = scenario.fixture();
        const record = { commands: github.commands, issues: github.issues, answer: '' };
        evidence(record);
        const worker = createGitHubWorker({
          ...github.settings(),
          model,
          thinkingLevel: thinkingSetting('CHATTO_AGENT_THINKING', 'medium')
        });
        record.answer = await worker(context, {
          task: scenario.request,
          evidence: scenario.evidence ?? ''
        });
        assert(
          github.commands.every((command) => command.supported),
          'Fixture encountered an unsupported command; inspect the report'
        );
        scenario.check(github, record.answer);
      });
    }
    for (const scenario of supervisorCases.filter((item) => selected(item.name))) {
      await evaluate(scenario.name, async (context, evidence) => {
        const result = await supervisor(context, model, scenario);
        evidence(result);
        const expected = scenario.messages.find(
          (message) => message.id === scenario.expectedRequest
        );
        assert.equal(result.delegated.length, expected ? 1 : 0, 'Incorrect worker delegation');
        assert.deepEqual(
          result.attempts.map((attempt) => attempt.requestId),
          expected ? [expected.id] : [],
          'Supervisor attempted to delegate the wrong message'
        );
        assert(
          result.posts.length > 0 || result.handled.length > 0,
          'Supervisor did not answer or react'
        );
        assert(
          !result.posts.includes('Sorry, I could not answer that. Please try again.'),
          'Supervisor turn failed'
        );
        if (scenario.automaticIntake && scenario.expectedHandled?.length === 0)
          assert(!result.handled.includes(true), 'Do not mark an unresolved request handled');
        else
          assert.deepEqual(
            result.handled,
            scenario.expectedHandled ?? [],
            'Unexpected handled reaction changes'
          );
        assert.deepEqual(
          result.investigationAttempts.map((attempt) => attempt.requestId),
          scenario.expectedInvestigation ? [scenario.expectedInvestigation] : [],
          'Supervisor attempted to investigate without the correct maintainer request'
        );
        assert.equal(result.investigations.length, scenario.expectedInvestigation ? 1 : 0);
        if (expected || scenario.expectedInvestigation) {
          assert.equal(result.posts.length, 2, 'Expected one acknowledgment and one result');
          assert.doesNotMatch(
            result.posts[0]!,
            /\b(filed|created|completed|finished|done)\b/i,
            'Acknowledgment must not claim completed work'
          );
        }
        assert.equal(result.directWrites, 0, 'Supervisor attempted a direct write');
        assert(
          result.commands.every((command) => command.supported),
          'Unsupported fixture command'
        );
        scenario.check?.(result.posts, result.commands, result.delegated);
      });
    }
    if (selected('language-report-withdrawal')) {
      await evaluate('language-report-withdrawal', (context, evidence) =>
        languageReport(context, model, evidence)
      );
    }
    const passed = results.filter((result) => result.passed).length;
    const report = resolve(directory, 'report.json');
    // Print the report location without printing fixture message bodies.
    log.info(`${passed}/${results.length} evaluations passed. Report: ${report}`);
    if (passed !== results.length) throw new Error(`GitHub evaluations failed. Report: ${report}`);
    return { passed, total: results.length, report };
  }
);
