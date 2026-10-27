import { createTaskContext } from './context.ts';
import { ansiColor } from './ansi.ts';
import { AsyncResource } from 'node:async_hooks';
import { beforeEach, describe, expect, test, vi } from 'vitest';
import { observeRunlingEvents, type RunlingEvent } from './events.ts';

const fakeModel = {
  id: 'claude-opus-4-5',
  name: 'Claude Opus 4.5',
  provider: 'anthropic'
};

const emptyUsage = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 };

let eventHandler: ((event: any) => void) | undefined;
let promptImplementation: ((prompt: string) => Promise<void>) | undefined;
let sessionOptions: any;
let resourceOptions: any;
let settingsCreateArgs: unknown[] | undefined;
let settingsOverrides: any;
let promptCalls = 0;
let sessionCreations = 0;
let createdSessions: any[] = [];
let abortCalls = 0;
let disposed = false;
let modelAvailable = true;
let modelAvailableAfterRefresh = false;
const refreshModels = vi.fn(
  async (_options: {
    providers: string[];
    allowNetwork: boolean;
    force: boolean;
    signal: AbortSignal;
  }) => {
    modelAvailable = modelAvailableAfterRefresh;
  }
);

vi.doMock('@earendil-works/pi-coding-agent', async () => {
  const actual = await vi.importActual<typeof import('@earendil-works/pi-coding-agent')>(
    '@earendil-works/pi-coding-agent'
  );
  return {
    convertToLlm: actual.convertToLlm,
    ModelRuntime: {
      create: async () => ({
        getModel: () => (modelAvailable ? fakeModel : undefined),
        refresh: refreshModels
      })
    },
    DefaultResourceLoader: class {
      constructor(options: unknown) {
        resourceOptions = options;
      }
      async reload() {}
    },
    getAgentDir: () => '/tmp/agent-dir',
    SettingsManager: {
      create: (...args: unknown[]) => {
        settingsCreateArgs = args;
        return {
          getRetrySettings: () => ({
            enabled: true,
            maxRetries: 3,
            baseDelayMs: 2000
          }),
          applyOverrides: (overrides: unknown) => {
            settingsOverrides = overrides;
          }
        };
      }
    },
    SessionManager: actual.SessionManager,
    createAgentSession: async (options: unknown) => {
      sessionCreations++;
      sessionOptions = options;
      let messages: any[] = sessionOptions.sessionManager.buildSessionContext().messages;
      const session = {
        bindExtensions: vi.fn(async () => {}),
        extensionRunner: { emit: vi.fn(async () => {}) },
        sessionManager: sessionOptions.sessionManager,
        agent: {
          toolExecution: 'parallel',
          steer: vi.fn(),
          clearSteeringQueue: vi.fn(),
          state: {
            get messages() {
              return messages;
            },
            set messages(next: any[]) {
              messages = [...next];
            }
          }
        },
        subscribe(handler: (event: any) => void) {
          eventHandler = handler;
          return () => {
            if (eventHandler === handler) eventHandler = undefined;
          };
        },
        prompt: async (prompt: string) => {
          promptCalls++;
          await promptImplementation?.(prompt);
        },
        abort: async () => {
          abortCalls++;
        },
        dispose: () => {
          disposed = true;
        }
      };
      createdSessions.push(session);
      return { session };
    },
    defineTool: (tool: unknown) => tool
  };
});

const {
  createAgentSession,
  AgentOutcomeError,
  answerText,
  describeTool,
  runAgent,
  toolFailureCategory
} = await import('./agent.ts');

test.each([
  ['ENOENT: private path', 'not_found'],
  ['permission denied: secret', 'permission_denied'],
  ['ETIMEDOUT: secret URL', 'timeout'],
  ['invalid arguments: secret', 'invalid_arguments'],
  ['Something else containing private data', 'unknown']
])('categorizes tool failures without forwarding output', (text, expected) => {
  expect(toolFailureCategory({ content: [{ type: 'text', text }] })).toBe(expected);
});

test('starts extensions, configures sequential tools, and waits for one shutdown', async () => {
  const instance = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5',
    output: 'text',
    tools: 'extensions',
    toolExecution: 'sequential'
  });
  const session = createdSessions[0];
  expect(sessionOptions.noTools).toBe('builtin');
  expect(sessionOptions.tools).toBeUndefined();
  expect(session.agent.toolExecution).toBe('sequential');
  expect(session.bindExtensions).toHaveBeenCalledOnce();
  const closed = Promise.withResolvers<void>();
  session.extensionRunner.emit.mockImplementation(() => closed.promise);
  const first = instance.dispose();
  const second = instance.dispose();
  expect(disposed).toBe(false);
  expect(session.extensionRunner.emit).toHaveBeenCalledExactlyOnceWith({
    type: 'session_shutdown',
    reason: 'quit'
  });
  closed.resolve();
  await Promise.all([first, second]);
  expect(disposed).toBe(true);
});

test('malformed failure details do not break activity reporting', () => {
  expect(toolFailureCategory({ content: [null, { type: 'text', text: 42 }] })).toBe('unknown');
  expect(toolFailureCategory(undefined)).toBe('unknown');
});

test('private agent content stays out of logs and events while results and usage remain available', async () => {
  const events: RunlingEvent[] = [];
  const output = vi.spyOn(console, 'info').mockImplementation(() => {});
  promptImplementation = async () => {
    eventHandler?.({
      type: 'auto_retry_start',
      delayMs: 100,
      attempt: 1,
      maxAttempts: 5,
      errorMessage: 'PRIVATE_PROVIDER_ERROR'
    });
    eventHandler?.({
      type: 'message_update',
      assistantMessageEvent: {
        type: 'text_delta',
        contentIndex: 0,
        partial: { content: [{ type: 'text', text: 'PRIVATE_PREVIEW' }] }
      }
    });
    eventHandler?.({
      type: 'tool_execution_start',
      toolName: 'bash',
      toolCallId: 'one',
      args: { command: 'PRIVATE_ARGUMENT' }
    });
    eventHandler?.({
      type: 'message_end',
      message: {
        role: 'assistant',
        stopReason: 'stop',
        content: [
          { type: 'thinking', thinking: 'PRIVATE_THOUGHT' },
          { type: 'text', text: 'PRIVATE_ANSWER' }
        ],
        usage: { ...emptyUsage, input: 5 }
      }
    });
  };
  try {
    const result = await observeRunlingEvents(
      (event) => events.push(event),
      () =>
        runAgent(createTaskContext(), 'PRIVATE_PROMPT', {
          cwd: '/project',
          model: 'anthropic/claude-opus-4-5',
          output: 'text',
          logContent: false
        })
    );
    expect(result.summary).toBe('PRIVATE_ANSWER');
    expect(result.usage.input).toBe(5);
    expect(JSON.stringify(events)).not.toContain('PRIVATE_');
    expect(JSON.stringify(output.mock.calls)).not.toContain('PRIVATE_');
    expect(events.some((event) => event.type === 'agent.tool')).toBe(true);
  } finally {
    output.mockRestore();
  }
});

const { log } = await import('./log.ts');

beforeEach(() => {
  eventHandler = undefined;
  promptImplementation = undefined;
  sessionOptions = undefined;
  resourceOptions = undefined;
  settingsCreateArgs = undefined;
  settingsOverrides = undefined;
  promptCalls = 0;
  sessionCreations = 0;
  createdSessions = [];
  abortCalls = 0;
  disposed = false;
  modelAvailable = true;
  modelAvailableAfterRefresh = false;
  refreshModels.mockClear();
  vi.unstubAllEnvs();
});

async function reportOutcome(report: {
  outcome: 'completed' | 'blocked' | 'failed';
  summary: string;
  details?: string;
}) {
  await sessionOptions.customTools[0].execute('tool-call', report);
}

function emitAssistantText(text: string) {
  eventHandler?.({
    type: 'message_end',
    message: { role: 'assistant', content: [{ type: 'text', text }] }
  });
}

function stripAnsi(text: string): string {
  return text.replace(/\x1b\[[0-9;]*m/g, '');
}

function emitAssistantUsage(usage: {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  cost?: { total: number };
}) {
  eventHandler?.({
    type: 'message_end',
    message: {
      role: 'assistant',
      content: [],
      usage
    }
  });
}

test('streams bounded text previews, throttles deltas, and shows tools without debug noise', async () => {
  const events: RunlingEvent[] = [];
  const clock = vi.spyOn(performance, 'now').mockReturnValue(1000);
  promptImplementation = async () => {
    eventHandler?.({ type: 'agent_start' });
    const update = (type: string, text: string) =>
      eventHandler?.({
        type: 'message_update',
        assistantMessageEvent: {
          type,
          contentIndex: 0,
          partial: { content: [{ type: 'text', text }] }
        }
      });
    update('text_start', '');
    update('text_delta', 'Looking\n at files');
    update('text_delta', 'This update is throttled');
    update('text_end', 'x'.repeat(600));
    eventHandler?.({
      type: 'tool_execution_start',
      toolCallId: 'read-1',
      toolName: 'read',
      args: { path: 'src/foo.ts' }
    });
    eventHandler?.({ type: 'turn_start' });
    await reportOutcome({ outcome: 'completed', summary: 'Done' });
  };
  try {
    await observeRunlingEvents(
      (e) => events.push(e),
      () =>
        runAgent(createTaskContext(), 'Test', {
          cwd: '/project',
          model: 'anthropic/claude-opus-4-5'
        })
    );
    const previews = events.filter((e) => e.type === 'agent.progress').map((e) => e.text);
    expect(previews).toContain('Looking at files');
    expect(previews).not.toContain('This update is throttled');
    expect(previews).toContain('x'.repeat(500));
    expect(previews).toContain('Reading src/foo.ts');
    expect(previews.at(-1)).toBe('Done');
  } finally {
    clock.mockRestore();
  }
});

test('previews report-only output such as the joke workflow', async () => {
  const events: RunlingEvent[] = [];
  // The SDK can dispatch from an async resource created outside the run.
  const sdk = new AsyncResource('sdk-dispatch');
  promptImplementation = async () => {
    sdk.runInAsyncScope(() => eventHandler?.({ type: 'agent_start' }));
    for (const type of ['toolcall_start', 'toolcall_delta', 'toolcall_end']) {
      sdk.runInAsyncScope(() =>
        eventHandler?.({
          type: 'message_update',
          assistantMessageEvent: {
            type,
            contentIndex: 0,
            partial: { content: [{ type: 'toolCall', name: 'report_outcome', arguments: {} }] }
          }
        })
      );
    }
    await sdk.runInAsyncScope(() =>
      reportOutcome({
        outcome: 'completed',
        summary: 'Joke written',
        details: 'A joke\nwith a punchline.'
      })
    );
  };
  await observeRunlingEvents(
    (e) => events.push(e),
    () =>
      runAgent(createTaskContext(), 'Write a joke', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5',
        tools: []
      })
  );
  const previews = events.filter((e) => e.type === 'agent.progress').map((e) => e.text);
  expect(previews.filter((text) => text === 'Preparing result…')).toHaveLength(1);
  expect(previews.at(-1)).toBe('A joke with a punchline.');
  expect(
    events.some((e) => e.type === 'agent.action' && e.action === 'A joke\nwith a punchline.')
  ).toBe(true);
});

describe('answerText', () => {
  const part = (text: string, phase?: string) => ({
    type: 'text',
    text,
    ...(phase ? { textSignature: JSON.stringify({ v: 1, id: text, phase }) } : {})
  });
  test('keeps only the final answer when the provider marks one', () => {
    expect(
      answerText([
        part('Checking the diff first.', 'commentary'),
        { type: 'thinking' },
        part('The diff is clean.', 'final_answer')
      ])
    ).toBe('The diff is clean.');
  });
  test('keeps all text when no part is marked as the final answer', () => {
    expect(answerText([part('One.'), part('Two.', 'commentary')])).toBe('One.\nTwo.');
    expect(answerText([{ type: 'text', text: 'Plain', textSignature: 'legacy-id' }])).toBe('Plain');
  });
});

describe('describeTool', () => {
  test('describes known tools', () => {
    expect(describeTool('read', { path: 'src/foo.ts' })).toBe('Reading src/foo.ts');
    expect(describeTool('edit', { path: 'src/foo.ts' })).toBe('Editing src/foo.ts');
    expect(describeTool('write', { path: 'src/foo.ts' })).toBe('Writing src/foo.ts');
    expect(describeTool('bash', { command: 'pnpm test' })).toBe('Running pnpm test');
    expect(describeTool('report_outcome', {})).toBe('Using report_outcome');
  });

  test('falls back to a generic message for unknown tools', () => {
    expect(describeTool('grep', { pattern: 'foo' })).toBe('Using grep');
  });
});

describe('runAgent', () => {
  test('uses a distinct, consistent color for each agent', async () => {
    const withColor = vi.spyOn(log, 'withColor');
    const colorize = vi.spyOn(log, 'colorize');
    promptImplementation = async () => {
      eventHandler?.({ type: 'agent_start' });
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    try {
      await runAgent(createTaskContext(), 'Do the first thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      });
      const firstAgentColors = withColor.mock.calls.map((call) => call[0]);
      withColor.mockClear();

      await runAgent(createTaskContext(), 'Do the second thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      });

      const secondAgentColors = withColor.mock.calls.map((call) => call[0]);
      expect(new Set(firstAgentColors).size).toBe(1);
      expect(new Set(secondAgentColors).size).toBe(1);
      expect(firstAgentColors[0]).not.toBe(secondAgentColors[0]);
      expect(colorize).toHaveBeenCalled();
      expect(colorize.mock.calls.every((call) => /^\[[a-z]+-[a-z]+-\d{4}\]$/.test(call[0]))).toBe(
        true
      );
    } finally {
      colorize.mockRestore();
      withColor.mockRestore();
    }
  });

  test('logs agent events without adding another step or indentation', async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);
    promptImplementation = async () => {
      eventHandler?.({ type: 'agent_start' });
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    try {
      await runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      });
    } finally {
      console.log = originalLog;
    }

    const started = logs.find((line) => line.includes('Agent started'));

    expect(logs.some((line) => /Agent [a-z]+-[a-z]+-\d{4}$/.test(line))).toBe(false);
    expect(started).toBeDefined();
    expect(started).not.toMatch(/^\s/);
  });

  test('prefixes every log line with one human-friendly agent ID', async () => {
    vi.stubEnv('NO_COLOR', undefined);
    vi.stubEnv('FORCE_COLOR', '1');
    const logs: string[] = [];
    const errors: string[] = [];
    const originalLog = console.log;
    const originalError = console.error;
    console.log = (message: string) => logs.push(message);
    console.error = (message: string) => errors.push(message);
    promptImplementation = async () => {
      eventHandler?.({ type: 'agent_start' });
      eventHandler?.({
        type: 'tool_execution_start',
        toolCallId: 'read-1',
        toolName: 'read',
        args: { path: 'src/foo.ts' }
      });
      eventHandler?.({
        type: 'tool_execution_end',
        toolCallId: 'bash-1',
        toolName: 'bash',
        isError: true
      });
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    try {
      await runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      });
    } finally {
      console.log = originalLog;
      console.error = originalError;
      vi.unstubAllEnvs();
    }

    const plainLogs = logs.map(stripAnsi);
    const plainErrors = errors.map(stripAnsi);
    const id = plainLogs
      .find((line) => line.includes('Agent started'))
      ?.match(/\[([a-z]+-[a-z]+-\d{4})\]/)?.[1];

    expect(id).toBeDefined();
    expect(plainLogs.some((line) => line.includes(`[${id}] Agent started`))).toBe(true);
    expect(plainLogs.some((line) => line.includes(`[${id}] Reading src/foo.ts`))).toBe(true);
    const reading = logs.find((line) => line.includes('Reading') && line.includes('src/foo.ts'));
    expect(reading).toBeDefined();
    expect(reading).toContain('\x1b[1m');
    expect(reading).toContain(ansiColor('#40c057') ?? '');
    expect(reading!.indexOf('\x1b[0m', reading!.indexOf('Reading'))).toBeLessThan(
      reading!.indexOf('src/foo.ts')
    );
    expect(plainErrors.some((line) => line.includes(`[${id}] bash failed`))).toBe(true);
  });

  test('logs automatic retry progress', async () => {
    const logs: string[] = [];
    const errors: string[] = [];
    const originalLog = console.log;
    const originalError = console.error;
    console.log = (message: string) => logs.push(stripAnsi(message));
    console.error = (message: string) => errors.push(stripAnsi(message));
    promptImplementation = async () => {
      eventHandler?.({
        type: 'auto_retry_start',
        attempt: 1,
        maxAttempts: 5,
        delayMs: 2000,
        errorMessage: 'fetch failed\nwhile streaming'
      });
      eventHandler?.({ type: 'auto_retry_end', success: true, attempt: 1 });
      eventHandler?.({
        type: 'auto_retry_end',
        success: false,
        attempt: 5,
        finalError: 'connection lost'
      });
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    try {
      await runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      });
    } finally {
      console.log = originalLog;
      console.error = originalError;
    }

    expect(
      logs.some((line) =>
        line.includes('Retrying agent in 2s (attempt 1/5): fetch failed while streaming')
      )
    ).toBe(true);
    expect(logs.some((line) => line.includes('Agent recovered after 1 retry attempt'))).toBe(true);
    expect(
      errors.some((line) =>
        line.includes('Agent retry failed after 5 retry attempts: connection lost')
      )
    ).toBe(true);
  });

  test('logs detailed turn, tool, and session lifecycle events', async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    const originalLevel = log.level;
    console.log = (message: string) => logs.push(stripAnsi(message));
    log.level = 'debug';
    promptImplementation = async () => {
      eventHandler?.({ type: 'turn_start' });
      eventHandler?.({
        type: 'tool_execution_start',
        toolCallId: 'read-1',
        toolName: 'read',
        args: { path: 'src/foo.ts' }
      });
      eventHandler?.({
        type: 'tool_execution_end',
        toolCallId: 'read-1',
        toolName: 'read',
        result: {},
        isError: false
      });
      eventHandler?.({
        type: 'turn_end',
        message: { role: 'assistant', content: [] },
        toolResults: [{}]
      });
      eventHandler?.({
        type: 'agent_end',
        messages: [{ role: 'assistant', content: [] }],
        willRetry: false
      });
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    try {
      await runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      });
    } finally {
      log.level = originalLevel;
      console.log = originalLog;
    }

    expect(logs.some((line) => line.includes('Turn 1 started'))).toBe(true);
    expect(logs.some((line) => line.includes('read finished in'))).toBe(true);
    expect(logs.some((line) => line.includes('Turn 1 finished (1 tool result)'))).toBe(true);
    expect(logs.some((line) => line.includes('Agent finished (1 message)'))).toBe(true);
  });

  test('exposes pi session events to callers', async () => {
    const events: string[] = [];
    promptImplementation = async () => {
      eventHandler?.({ type: 'agent_start' });
      eventHandler?.({ type: 'turn_start' });
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    await runAgent(createTaskContext(), 'Do the thing', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5',
      onEvent: (event) => events.push(event.type)
    });

    expect(events).toEqual(['agent_start', 'turn_start']);
  });

  test('logs compaction and summarization retry progress', async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(stripAnsi(message));
    promptImplementation = async () => {
      eventHandler?.({ type: 'compaction_start', reason: 'threshold' });
      eventHandler?.({
        type: 'summarization_retry_scheduled',
        attempt: 1,
        maxAttempts: 5,
        delayMs: 1500,
        errorMessage: 'stream terminated\nwhile summarizing'
      });
      eventHandler?.({
        type: 'compaction_end',
        reason: 'threshold',
        result: {
          summary: 'Earlier work',
          firstKeptEntryId: 'entry-1',
          tokensBefore: 120000,
          estimatedTokensAfter: 30000
        },
        aborted: false,
        willRetry: false
      });
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    try {
      await runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      });
    } finally {
      console.log = originalLog;
    }

    expect(logs.some((line) => line.includes('Compacting context (threshold)'))).toBe(true);
    expect(
      logs.some((line) =>
        line.includes(
          'Retrying context summary in 1500ms (attempt 1/5): stream terminated while summarizing'
        )
      )
    ).toBe(true);
    expect(
      logs.some((line) => line.includes('Context compacted from 120,000 to about 30,000 tokens'))
    ).toBe(true);
  });

  test("raises Pi's agent retry budget to at least five", async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    await runAgent(createTaskContext(), 'Do the thing', {
      model: 'anthropic/claude-opus-4-5',
      cwd: '/tmp/project',
      resources: { agentDir: '/tmp/custom-agent' }
    });

    expect(settingsCreateArgs).toEqual(['/tmp/project', '/tmp/custom-agent']);
    expect(settingsOverrides).toEqual({
      retry: { enabled: true, maxRetries: 5, baseDelayMs: 2000 }
    });
    expect(resourceOptions.settingsManager).toBe(sessionOptions.settingsManager);
  });

  test('a session file keeps the conversation for a later createAgentSession, but not for a fork', async () => {
    const { mkdtemp, readFile, rm } = await import('node:fs/promises');
    const { tmpdir } = await import('node:os');
    const { join } = await import('node:path');
    const directory = await mkdtemp(join(tmpdir(), 'runling-session-'));
    const sessionFile = join(directory, 'worker.jsonl');
    try {
      const first = await createAgentSession({
        model: 'anthropic/claude-opus-4-5',
        cwd: '.',
        sessionFile
      });
      const manager = sessionOptions.sessionManager;
      manager.appendMessage({ role: 'user', content: 'Add /shrug', timestamp: 1 });
      manager.appendMessage({
        role: 'assistant',
        content: [{ type: 'text', text: 'Reading the composer.' }],
        api: 'anthropic-messages',
        provider: 'anthropic',
        model: 'claude-opus-4-5',
        usage: {
          ...emptyUsage,
          totalTokens: 0,
          cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 }
        },
        stopReason: 'stop',
        timestamp: 2
      });
      first.dispose();
      expect(await readFile(sessionFile, 'utf8')).toContain('Reading the composer.');

      const resumed = await createAgentSession({
        model: 'anthropic/claude-opus-4-5',
        cwd: '.',
        sessionFile
      });
      expect(
        sessionOptions.sessionManager
          .buildSessionContext()
          .messages.map((message: { role: string }) => message.role)
      ).toEqual(['user', 'assistant']);
      const fork = await resumed.fork();
      expect(sessionOptions.sessionManager.getSessionFile()).toBeUndefined();
      fork.dispose();
      resumed.dispose();
      await expect(
        createAgentSession({
          model: 'anthropic/claude-opus-4-5',
          cwd: '.',
          sessionFile,
          trust: { untrusted: ['fetch'], blockAfterUntrusted: ['write'] }
        })
      ).rejects.toThrow('cannot be combined with a trust policy');
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  });

  test('returns the structured outcome reported by the agent', async () => {
    promptImplementation = async () => {
      await reportOutcome({
        outcome: 'blocked',
        summary: 'Need access',
        details: 'The required repository is unavailable.'
      });
    };

    await expect(
      runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      })
    ).resolves.toEqual({
      outcome: 'blocked',
      summary: 'Need access',
      details: 'The required repository is unavailable.',
      usage: emptyUsage
    });
    expect(disposed).toBe(true);
  });

  test('does not treat an unreported plain-text response as success', async () => {
    promptImplementation = async () => emitAssistantText('I could not finish');

    await expect(
      runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      })
    ).resolves.toEqual({
      outcome: 'failed',
      summary: 'Agent finished without a valid outcome report',
      failureReason: 'missing_outcome',
      usage: emptyUsage
    });
    expect(promptCalls).toBe(2);
  });

  test('recovers a malformed tool call with one corrective turn', async () => {
    promptImplementation = async () => {
      if (promptCalls === 1) {
        emitAssistantText('functions.report_outcome:0{"outcome":"completed"}');
      } else {
        await reportOutcome({ outcome: 'completed', summary: 'Done' });
      }
    };

    await expect(
      runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      })
    ).resolves.toEqual({
      outcome: 'completed',
      summary: 'Done',
      usage: emptyUsage
    });
    expect(promptCalls).toBe(2);
  });

  test('returns and logs accumulated token usage including cached tokens', async () => {
    const logs: string[] = [];
    const originalLog = console.log;
    console.log = (message: string) => logs.push(message);
    promptImplementation = async () => {
      emitAssistantUsage({
        input: 100,
        output: 20,
        cacheRead: 500,
        cacheWrite: 10
      });
      emitAssistantUsage({
        input: 50,
        output: 25,
        cacheRead: 550,
        cacheWrite: 15
      });
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    try {
      await expect(
        runAgent(createTaskContext(), 'Do the thing', {
          cwd: '/project',
          model: 'anthropic/claude-opus-4-5'
        })
      ).resolves.toEqual({
        outcome: 'completed',
        summary: 'Done',
        usage: {
          input: 150,
          costIncomplete: true,
          output: 45,
          cacheRead: 1050,
          cacheWrite: 25
        }
      });
    } finally {
      console.log = originalLog;
    }

    expect(
      logs.some((line) =>
        line.includes('Token usage: in 150, out 45, cache read 1,050, cache write 25')
      )
    ).toBe(true);
  });

  test('records usage into the workflow-wide totals', async () => {
    const ctx = createTaskContext();
    promptImplementation = async () => {
      emitAssistantUsage({ input: 10, output: 5, cacheRead: 0, cacheWrite: 0 });
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    await runAgent(ctx, 'Do the thing', { cwd: '/project', model: 'anthropic/claude-opus-4-5' });

    expect(ctx.usage).toEqual({
      input: 10,
      output: 5,
      cacheRead: 0,
      cacheWrite: 0,
      costIncomplete: true
    });
  });

  test('emits immutable live usage snapshots before an agent finishes', async () => {
    const events: RunlingEvent[] = [];
    promptImplementation = async () => {
      emitAssistantUsage({
        input: 10,
        output: 2,
        cacheRead: 5,
        cacheWrite: 0,
        cost: { total: 0.01 }
      });
      expect(events.filter((e) => e.type === 'agent.usage')).toHaveLength(1);
      expect(events.some((e) => e.type === 'agent.finished')).toBe(false);
      emitAssistantUsage({
        input: 20,
        output: 3,
        cacheRead: 7,
        cacheWrite: 1,
        cost: { total: 0.02 }
      });
      throw new Error('Provider unavailable');
    };
    await expect(
      observeRunlingEvents(
        (e) => events.push(e),
        () =>
          runAgent(createTaskContext(), 'Do the thing', {
            cwd: '/project',
            model: 'anthropic/claude-opus-4-5'
          })
      )
    ).rejects.toThrow('Provider unavailable');
    const snapshots = events.filter((e) => e.type === 'agent.usage');
    expect(snapshots.map((e) => e.usage.input)).toEqual([10, 30]);
    expect(snapshots[1]?.usage.cost).toBeCloseTo(0.03);
    const finished = events.find((e) => e.type === 'agent.finished');
    expect(finished?.usage).toEqual(snapshots[1]?.usage);
  });

  test('records tokens and cost as each model turn completes', async () => {
    const ctx = createTaskContext();
    promptImplementation = async () => {
      emitAssistantUsage({
        input: 10,
        output: 5,
        cacheRead: 20,
        cacheWrite: 1,
        cost: { total: 0.0125 }
      });
      expect(ctx.usage).toEqual({
        input: 10,
        output: 5,
        cacheRead: 20,
        cacheWrite: 1,
        cost: 0.0125
      });
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    await runAgent(ctx, 'Do the thing', { cwd: '/project', model: 'anthropic/claude-opus-4-5' });
  });

  test('returns token usage even when no outcome is reported', async () => {
    promptImplementation = async () => {
      if (promptCalls === 1) {
        emitAssistantUsage({
          input: 7,
          output: 3,
          cacheRead: 0,
          cacheWrite: 0
        });
        emitAssistantText('I could not finish');
      }
    };

    await expect(
      runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      })
    ).resolves.toEqual({
      outcome: 'failed',
      summary: 'Agent finished without a valid outcome report',
      failureReason: 'missing_outcome',
      usage: {
        input: 7,
        output: 3,
        cacheRead: 0,
        cacheWrite: 0,
        costIncomplete: true
      }
    });
  });

  test('disposes the session when prompting fails', async () => {
    promptImplementation = async () => {
      throw new Error('Provider unavailable');
    };

    await expect(
      runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      })
    ).rejects.toThrow('Provider unavailable');
    expect(disposed).toBe(true);
  });

  test('records usage when prompting fails', async () => {
    const ctx = createTaskContext();
    promptImplementation = async () => {
      emitAssistantUsage({
        input: 10,
        output: 5,
        cacheRead: 20,
        cacheWrite: 1
      });
      throw new Error('Provider unavailable');
    };

    await expect(
      runAgent(ctx, 'Do the thing', { cwd: '/project', model: 'anthropic/claude-opus-4-5' })
    ).rejects.toThrow('Provider unavailable');
    expect(ctx.usage).toEqual({
      input: 10,
      output: 5,
      cacheRead: 20,
      cacheWrite: 1,
      costIncomplete: true
    });
  });

  test('discovers a missing model by refreshing only its provider', async () => {
    modelAvailable = false;
    modelAvailableAfterRefresh = true;
    await using agent = await createAgentSession({
      cwd: '/project',
      model: 'openrouter/deepseek/deepseek-v4.1-flash-1017'
    });
    expect(refreshModels).toHaveBeenCalledExactlyOnceWith({
      providers: ['openrouter'],
      allowNetwork: true,
      force: true,
      signal: expect.any(AbortSignal)
    });
    expect(sessionCreations).toBe(1);
  });

  test('known models need no catalog refresh', async () => {
    await using agent = await createAgentSession({
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });
    expect(refreshModels).not.toHaveBeenCalled();
  });

  test('offline mode rejects a missing model without network discovery', async () => {
    vi.stubEnv('PI_OFFLINE', '1');
    modelAvailable = false;
    await expect(
      createAgentSession({
        cwd: '/project',
        model: 'openrouter/missing'
      })
    ).rejects.toThrow('Model openrouter/missing is unavailable');
    expect(refreshModels).not.toHaveBeenCalled();
    expect(sessionOptions).toBeUndefined();
  });

  test('rejects unavailable models before creating a session', async () => {
    modelAvailable = false;

    await expect(
      runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/missing'
      })
    ).rejects.toThrow('Model anthropic/missing is unavailable');
    expect(sessionOptions).toBeUndefined();
  });

  test('forwards the requested thinking level to the session', async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    await runAgent(createTaskContext(), 'Do the thing', {
      cwd: '/project',
      model: 'openai-codex/gpt-5.6-sol',
      thinkingLevel: 'medium'
    });

    expect(sessionOptions.thinkingLevel).toBe('medium');
  });

  test('leaves the thinking level to pi defaults when unspecified', async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    await runAgent(createTaskContext(), 'Do the thing', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });

    expect(sessionOptions.thinkingLevel).toBeUndefined();
  });

  test("loads and enables Runling's bundled web fetch extension", async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    await runAgent(createTaskContext(), 'Research the topic', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });

    expect(sessionOptions.tools).toEqual([
      'read',
      'bash',
      'edit',
      'write',
      'web_fetch',
      'report_outcome'
    ]);
    expect(resourceOptions.extensionFactories).toEqual([
      {
        name: 'runling-web-fetch',
        factory: expect.any(Function)
      }
    ]);
  });

  test('loads explicitly supplied extensions when configured extensions are disabled', async () => {
    const localExtension = {
      name: 'local-validation',
      factory() {}
    };
    promptImplementation = async () => {
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };

    await runAgent(createTaskContext(), 'Implement the change', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5',
      extensions: [localExtension],
      resources: { extensions: false }
    });

    expect(resourceOptions.noExtensions).toBe(true);
    expect(resourceOptions.extensionFactories).toEqual([localExtension]);
    expect(sessionOptions.tools).not.toContain('web_fetch');
  });

  test('enables only explicit web tools with resource discovery disabled', async () => {
    const { default: webFetchExtension } = await import('../../extensions/web-fetch.ts');
    const extension = { name: 'runling-web-fetch', factory: webFetchExtension };
    promptImplementation = async () => {
      await reportOutcome({ outcome: 'completed', summary: 'Fetched' });
    };

    await runAgent(createTaskContext(), 'Read a public URL', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5',
      tools: ['web_fetch'],
      extensions: [extension],
      resources: {
        extensions: false,
        skills: false,
        promptTemplates: false,
        themes: false,
        contextFiles: false
      }
    });

    expect(sessionOptions.tools).toEqual(['web_fetch', 'report_outcome']);
    expect(resourceOptions).toMatchObject({
      extensionFactories: [extension],
      noExtensions: true,
      noSkills: true,
      noPromptTemplates: true,
      noThemes: true,
      noContextFiles: true
    });
  });

  test('applies execution and resource boundaries', async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: 'completed', summary: 'Reviewed' });
    };

    await runAgent(createTaskContext(), 'Review', {
      model: 'anthropic/claude-opus-4-5',
      cwd: '/tmp/project',
      tools: ['read'],
      resources: {
        agentDir: '/tmp/agent',
        extensions: false,
        skills: ['./skills'],
        promptTemplates: false,
        themes: false,
        contextFiles: false
      }
    });

    expect(sessionOptions.cwd).toBe('/tmp/project');
    expect(sessionOptions.tools).toEqual(['read', 'report_outcome']);
    expect(resourceOptions).toMatchObject({
      cwd: '/tmp/project',
      agentDir: '/tmp/agent',
      noExtensions: true,
      noSkills: true,
      additionalSkillPaths: ['./skills'],
      noPromptTemplates: true,
      noThemes: true,
      noContextFiles: true,
      extensionFactories: []
    });
    expect(resourceOptions.appendSystemPromptOverride(['Base prompt'])).toEqual([
      'Base prompt',
      expect.stringContaining('non-interactive software factory')
    ]);
  });

  test('aborts and disposes an active session', async () => {
    const controller = new AbortController();
    promptImplementation = async () => controller.abort('Stopped');

    await expect(
      runAgent(createTaskContext(), 'Do the thing', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5',
        signal: controller.signal
      })
    ).rejects.toBe('Stopped');
    expect(abortCalls).toBe(1);
    expect(disposed).toBe(true);
  });
});

describe('agent', () => {
  test('forks the current conversation into an independent session', async () => {
    const instance = await createAgentSession({
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5',
      tools: ['read']
    });
    createdSessions[0].agent.state.messages = [
      { role: 'user', content: [{ type: 'text', text: 'Investigate' }] }
    ];

    const fork = await instance.fork();

    try {
      expect(fork.id).not.toBe(instance.id);
      expect(sessionCreations).toBe(2);
      expect(createdSessions[1].agent.state.messages).toEqual(
        createdSessions[0].agent.state.messages
      );
      expect(createdSessions[1].sessionManager.buildSessionContext().messages).toEqual(
        createdSessions[0].agent.state.messages
      );

      createdSessions[1].agent.state.messages.push({ role: 'assistant' });
      expect(createdSessions[0].agent.state.messages).toHaveLength(1);
      expect(createdSessions[1].agent.state.messages).toHaveLength(2);
      expect(sessionOptions.tools).toEqual(['read', 'report_outcome']);
    } finally {
      fork.dispose();
      instance.dispose();
    }
  });

  test('blocks selected tools after untrusted content, including in forks', async () => {
    const onBlocked = vi.fn();
    const installTrust = () => {
      const handlers = new Map<string, (event: any) => any>();
      const trust = resourceOptions.extensionFactories.find(
        (extension: { name: string }) => extension.name === 'runling-trust'
      );
      trust.factory({
        on: (name: string, handler: (event: any) => any) => handlers.set(name, handler)
      });
      return {
        call: async (toolName: string) =>
          await handlers.get('tool_call')!({ type: 'tool_call', toolName, input: {} }),
        result: (toolName: string, isError = false) =>
          handlers.get('tool_result')!({ type: 'tool_result', toolName, isError, content: [] })
      };
    };
    const instance = await createAgentSession({
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5',
      tools: ['search', 'implement', 'read'],
      trust: {
        untrusted: ['search'],
        blockAfterUntrusted: ['implement'],
        onBlocked: async (toolName) => {
          onBlocked(toolName);
          throw new Error('Host notice failed');
        }
      }
    });
    const tools = installTrust();
    try {
      expect(await tools.call('implement')).toBeUndefined();
      tools.result('read');
      expect(await tools.call('implement')).toBeUndefined();
      tools.result('search', true);
      const events: RunlingEvent[] = [];
      const blockedCall = await observeRunlingEvents(
        (event) => events.push(event),
        () => tools.call('implement')
      );
      expect(blockedCall).toMatchObject({
        block: true,
        reason: expect.stringContaining('untrusted content')
      });
      expect(events).toContainEqual(
        expect.objectContaining({
          type: 'agent.action',
          action: expect.stringContaining('Blocked-tool callback failed for implement')
        })
      );
      expect(JSON.stringify(events)).not.toContain('Host notice failed');
      expect(onBlocked).toHaveBeenCalledWith('implement');
      expect(await tools.call('read')).toBeUndefined();
      expect(onBlocked).toHaveBeenCalledOnce();

      const fork = await instance.fork();
      try {
        expect(await installTrust().call('implement')).toMatchObject({ block: true });
      } finally {
        fork.dispose();
      }
    } finally {
      instance.dispose();
    }
  });

  test('does not install the trust extension without a policy', async () => {
    const instance = await createAgentSession({
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });
    try {
      expect(
        resourceOptions.extensionFactories.some(
          (extension: { name: string }) => extension.name === 'runling-trust'
        )
      ).toBe(false);
    } finally {
      instance.dispose();
    }
  });

  test('keeps inherited summaries in Pi history across further compaction', async () => {
    const instance = await createAgentSession({
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });
    createdSessions[0].agent.state.messages = [
      {
        role: 'compactionSummary',
        summary: 'Investigation found a race condition',
        tokensBefore: 5000,
        timestamp: 1
      },
      { role: 'user', content: 'Review the race condition', timestamp: 2 }
    ];
    const fork = await instance.fork();
    try {
      const manager = createdSessions[1].sessionManager;
      // Pi's compactor reads the persisted branch, not agent.state.messages.
      const branch = manager.getBranch();
      expect(JSON.stringify(branch)).toContain('Investigation found a race condition');
      expect(JSON.stringify(branch)).toContain('Review the race condition');
      const inherited = branch.find((entry: any) => entry.type === 'message');
      manager.appendMessage({ role: 'user', content: 'Continue the review', timestamp: 3 });
      manager.appendCompaction('Earlier work', inherited.id, 6000);
      const restored = manager.buildSessionContext().messages;
      expect(JSON.stringify(restored)).toContain('Investigation found a race condition');
      expect(JSON.stringify(restored)).toContain('Continue the review');
      expect(createdSessions[0].agent.state.messages).toHaveLength(2);
    } finally {
      fork.dispose();
      instance.dispose();
    }
  });

  test('does not fork a running or disposed agent', async () => {
    let instance: Awaited<ReturnType<typeof createAgentSession>>;
    promptImplementation = async () => {
      await expect(instance.fork()).rejects.toThrow('already running');
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };
    instance = await createAgentSession({ cwd: '/project', model: 'anthropic/claude-opus-4-5' });

    await instance.run(createTaskContext(), 'Work');
    instance.dispose();

    await expect(instance.fork()).rejects.toThrow('has been disposed');
  });

  test('retains one session across sequential runs', async () => {
    promptImplementation = async (prompt) => {
      await reportOutcome({ outcome: 'completed', summary: prompt });
    };
    const instance = await createAgentSession({
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });

    try {
      expect(instance.id).toMatch(/^[a-z]+-[a-z]+-\d{4}$/);
      await expect(instance.run(createTaskContext(), 'Investigate')).resolves.toMatchObject({
        outcome: 'completed',
        summary: 'Investigate'
      });
      await expect(instance.run(createTaskContext(), 'Implement')).resolves.toMatchObject({
        outcome: 'completed',
        summary: 'Implement'
      });

      expect(sessionCreations).toBe(1);
      expect(promptCalls).toBe(2);
      expect(disposed).toBe(false);
    } finally {
      await instance.dispose();
    }

    expect(disposed).toBe(true);
  });

  test('uses fresh outcome state for every run', async () => {
    promptImplementation = async (prompt) => {
      await reportOutcome(
        prompt === 'First'
          ? { outcome: 'completed', summary: 'First result' }
          : { outcome: 'blocked', summary: 'Second result' }
      );
    };
    const instance = await createAgentSession({
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });

    try {
      await expect(instance.run(createTaskContext(), 'First')).resolves.toMatchObject({
        outcome: 'completed',
        summary: 'First result'
      });
      await expect(instance.runOutcome(createTaskContext(), 'Second')).resolves.toMatchObject({
        outcome: 'blocked',
        summary: 'Second result'
      });
    } finally {
      instance.dispose();
    }
  });

  test('rejects concurrent runs', async () => {
    let release: (() => void) | undefined;
    promptImplementation = async () => {
      await new Promise<void>((resolve) => {
        release = resolve;
      });
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };
    const instance = await createAgentSession({
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });

    try {
      const first = instance.run(createTaskContext(), 'First');
      await expect(instance.run(createTaskContext(), 'Second')).rejects.toThrow('already running');
      release?.();
      await first;
    } finally {
      instance.dispose();
    }
  });

  test('rejects runs after disposal', async () => {
    const instance = await createAgentSession({
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });
    instance.dispose();

    await expect(instance.run(createTaskContext(), 'Too late')).rejects.toThrow(
      'has been disposed'
    );
  });

  test('can run again after an aborted turn', async () => {
    const controller = new AbortController();
    promptImplementation = async () => controller.abort('Stopped');
    const instance = await createAgentSession({
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });

    try {
      await expect(
        instance.run(createTaskContext(), 'First', { signal: controller.signal })
      ).rejects.toBe('Stopped');
      expect(disposed).toBe(false);

      promptImplementation = async () => {
        await reportOutcome({ outcome: 'completed', summary: 'Recovered' });
      };
      await expect(instance.run(createTaskContext(), 'Second')).resolves.toMatchObject({
        outcome: 'completed',
        summary: 'Recovered'
      });
    } finally {
      instance.dispose();
    }
  });

  test('supports automatic disposal', async () => {
    {
      await using instance = await createAgentSession({
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      });
      expect(disposed).toBe(false);
      expect(instance.id).toBeTypeOf('string');
    }

    expect(disposed).toBe(true);
  });

  test('throws unsuccessful outcomes while preserving their report', async () => {
    promptImplementation = async () => {
      await reportOutcome({ outcome: 'blocked', summary: 'Need input' });
    };
    await using instance = await createAgentSession({
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });

    try {
      await instance.run(createTaskContext(), 'Do the thing');
      throw new Error('Expected run to fail');
    } catch (error) {
      expect(error).toBeInstanceOf(AgentOutcomeError);
      expect((error as Error).message).toBe('Need input');
      expect((error as { report: unknown }).report).toEqual({
        outcome: 'blocked',
        summary: 'Need input',
        usage: emptyUsage
      });
    }
  });
});

test('requires an explicit directory before creating an agent', async () => {
  // @ts-expect-error Agents require an explicit directory.
  await expect(createAgentSession({ model: 'anthropic/claude-opus-4-5' })).rejects.toThrow(
    'An explicit directory is required'
  );
  await expect(createAgentSession({ model: 'anthropic/claude-opus-4-5', cwd: '' })).rejects.toThrow(
    'An explicit directory is required'
  );
});

test('attributes each reusable agent interaction to its supplied context', async () => {
  const first = createTaskContext();
  const second = createTaskContext();
  promptImplementation = async () => {
    emitAssistantUsage({ ...emptyUsage, input: 10, cost: { total: 0.25 } });
    await reportOutcome({ outcome: 'completed', summary: 'Done' });
  };
  const instance = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5'
  });
  try {
    const one = await instance.run(first, 'First');
    const two = await instance.runOutcome(second, 'Second');
    expect(one.usage.input).toBe(10);
    expect(two.usage.input).toBe(10);
    expect(first.usage).toEqual({ ...emptyUsage, input: 10, cost: 0.25 });
    expect(second.usage).toEqual(first.usage);
    await instance.run(second, 'Third');
    expect(first.usage.input).toBe(10);
    expect(second.usage).toEqual({ ...emptyUsage, input: 20, cost: 0.5 });
  } finally {
    instance.dispose();
  }
});

test('records fork usage only in the context passed to the fork interaction', async () => {
  const parentContext = createTaskContext();
  const childContext = createTaskContext();
  promptImplementation = async () => {
    emitAssistantUsage({ ...emptyUsage, input: 10, cost: { total: 0.25 } });
    await reportOutcome({ outcome: 'completed', summary: 'Done' });
  };
  const instance = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5'
  });
  try {
    await instance.run(parentContext, 'Investigate');
    const fork = await instance.fork();
    try {
      await fork.run(childContext, 'Review');
      expect(parentContext.usage.input).toBe(10);
      expect(childContext.usage.input).toBe(10);
      await fork.run(parentContext, 'Review again');
      expect(parentContext.usage.input).toBe(20);
      expect(childContext.usage.input).toBe(10);
    } finally {
      fork.dispose();
    }
  } finally {
    instance.dispose();
  }
});

test('retains usage recorded before cancellation without counting it again', async () => {
  const ctx = createTaskContext();
  const controller = new AbortController();
  promptImplementation = async () => {
    emitAssistantUsage({ ...emptyUsage, input: 10, cost: { total: 0.25 } });
    controller.abort('Stopped');
  };
  await expect(
    runAgent(ctx, 'Work', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5',
      signal: controller.signal
    })
  ).rejects.toBe('Stopped');
  expect(ctx.usage).toEqual({ ...emptyUsage, input: 10, cost: 0.25 });
});

test('context abort stops an active agent even with a separate call signal', async () => {
  const ctx = createTaskContext();
  const external = new AbortController();
  promptImplementation = async () => {
    emitAssistantUsage({ ...emptyUsage, input: 10, cost: { total: 0.25 } });
    // Catch the task's throw to check that the agent also observes the signal.
    try {
      ctx.abort('Budget exceeded');
    } catch {}
  };
  await expect(
    runAgent(ctx, 'Work', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5',
      signal: external.signal
    })
  ).rejects.toThrow('Budget exceeded');
  expect(abortCalls).toBe(1);
  expect(disposed).toBe(true);
  expect(external.signal.aborted).toBe(false);
  expect(ctx.usage).toEqual({ ...emptyUsage, input: 10, cost: 0.25 });
});

test('does not create an agent for an aborted context', async () => {
  const ctx = createTaskContext();
  try {
    ctx.abort('Already stopped');
  } catch {}
  await expect(
    runAgent(ctx, 'Work', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    })
  ).rejects.toBe(ctx.signal.reason);
  expect(sessionCreations).toBe(0);
});

test('reusable agents reject an aborted context but accept a fresh context', async () => {
  const stopped = createTaskContext();
  const fresh = createTaskContext();
  const instance = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5'
  });
  try {
    promptImplementation = async () => {
      try {
        stopped.abort('Stop');
      } catch {}
    };
    await expect(instance.run(stopped, 'First')).rejects.toBe(stopped.signal.reason);
    expect(disposed).toBe(false);
    await expect(instance.runOutcome(stopped, 'Again')).rejects.toBe(stopped.signal.reason);
    expect(promptCalls).toBe(1);
    promptImplementation = async () => {
      await reportOutcome({ outcome: 'completed', summary: 'Done' });
    };
    await expect(instance.run(fresh, 'Next')).resolves.toMatchObject({ summary: 'Done' });
    expect(fresh.signal.aborted).toBe(false);
  } finally {
    instance.dispose();
  }
});

test('per-call cancellation leaves the workflow context usable', async () => {
  const ctx = createTaskContext();
  const external = new AbortController();
  promptImplementation = async () => {
    external.abort('This call only');
  };
  await expect(
    runAgent(ctx, 'Work', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5',
      signal: external.signal
    })
  ).rejects.toBe('This call only');
  expect(ctx.signal.aborted).toBe(false);
});

test('steers an active interaction and invalidates its earlier report on delivery', async () => {
  const gate = Promise.withResolvers<void>();
  promptImplementation = async () => {
    if (promptCalls === 1) {
      await reportOutcome({ outcome: 'completed', summary: 'Old plan' });
      await gate.promise;
    } else {
      await reportOutcome({ outcome: 'completed', summary: 'Revised plan' });
    }
  };
  const instance = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5'
  });
  expect(await instance.steer('idle')).toBe(false);
  const run = instance.runOutcome(createTaskContext(), 'Plan');
  await vi.waitFor(() => expect(promptCalls).toBe(1));
  const delivery = instance.steer('Include tests');
  const message = createdSessions[0].agent.steer.mock.calls[0][0];
  expect(message.content).toEqual([{ type: 'text', text: 'Include tests' }]);
  eventHandler?.({ type: 'message_start', message });
  expect(await delivery).toBe(true);
  gate.resolve();
  expect(await run).toMatchObject({ summary: 'Revised plan' });
  expect(promptCalls).toBe(2);
  instance.dispose();
  expect(await instance.steer('disposed')).toBe(false);
});

test('returns undelivered steering at completion and does not carry it to another context', async () => {
  const gate = Promise.withResolvers<void>();
  promptImplementation = async () => {
    await gate.promise;
    await reportOutcome({ outcome: 'completed', summary: 'Done' });
  };
  const instance = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5'
  });
  const run = instance.runOutcome(createTaskContext(), 'First');
  const queued = instance.steer('Late feedback');
  eventHandler?.({ type: 'agent_end', messages: [] });
  expect(await instance.steer('Too late')).toBe(false);
  gate.resolve();
  await run;
  expect(await queued).toBe(false);
  expect(createdSessions[0].agent.clearSteeringQueue).toHaveBeenCalledOnce();
  await instance.runOutcome(createTaskContext(), 'Second');
  expect(createdSessions[0].agent.steer).toHaveBeenCalledOnce();
  instance.dispose();
});

test('settles pending steering on disposal and refuses steering after cancellation', async () => {
  const gate = Promise.withResolvers<void>();
  promptImplementation = () => gate.promise;
  const instance = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5'
  });
  const ctx = createTaskContext();
  const run = instance.runOutcome(ctx, 'Plan');
  const rejected = expect(run).rejects.toThrow('stop');
  const pending = instance.steer('Feedback');
  try {
    ctx.abort('stop');
  } catch {
    /* expected */
  }
  expect(await instance.steer('after cancellation')).toBe(false);
  instance.dispose();
  expect(await pending).toBe(false);
  gate.resolve();
  await rejected;
});

test('forwards completed assistant text during the interaction, excluding reasoning and tools', async () => {
  const gate = Promise.withResolvers<void>();
  promptImplementation = async () => {
    await gate.promise;
    await reportOutcome({ outcome: 'completed', summary: 'Done' });
  };
  const instance = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5'
  });
  const onText = vi.fn();
  const run = instance.runOutcome(createTaskContext(), 'Plan', { onText });
  eventHandler?.({
    type: 'message_end',
    message: {
      role: 'assistant',
      content: [
        { type: 'thinking', thinking: 'Private reasoning' },
        { type: 'text', text: 'Here is the joke.' },
        { type: 'toolCall', id: 'tool', name: 'read', arguments: {} }
      ],
      usage: emptyUsage
    }
  });
  eventHandler?.({
    type: 'message_end',
    message: { role: 'toolResult', content: [{ type: 'text', text: 'file contents' }] }
  });
  expect(onText.mock.calls).toEqual([['Here is the joke.']]);
  gate.resolve();
  await run;
  await instance.runOutcome(createTaskContext(), 'Next');
  expect(onText).toHaveBeenCalledOnce();
  instance.dispose();
});

test('retains earlier findings when a later report refers back to them, without leaking across interactions', async () => {
  promptImplementation = async () => {
    await reportOutcome({
      outcome: 'completed',
      summary: 'Found locale files',
      details: 'Edit locales.ts and messages/bar.json.'
    });
    await reportOutcome({ outcome: 'completed', summary: 'See the preceding outcome.' });
  };
  await using worker = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5'
  });
  const result = await worker.runOutcome(createTaskContext(), 'Investigate');
  expect(result.summary).toBe('See the preceding outcome.');
  expect(result.details).toContain('Edit locales.ts and messages/bar.json.');
  expect(result.details).toContain('latest; supersedes earlier conclusions');

  promptImplementation = async () => {
    await reportOutcome({ outcome: 'completed', summary: 'Unrelated task' });
  };
  const next = await worker.runOutcome(createTaskContext(), 'Other');
  expect(next.details).toBeUndefined();
});

test('custom system prompt replaces the coding persona and retains workflow instructions', async () => {
  await using instance = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5',
    systemPrompt: 'You are a chat assistant.',
    instructions: ['Keep replies brief.']
  });
  expect(resourceOptions.systemPromptOverride('Coding instructions')).toBe(
    'You are a chat assistant.'
  );
  const appended = resourceOptions.appendSystemPromptOverride([]).join('\n');
  expect(appended).toContain('Keep replies brief.');
  expect(appended).toContain('non-interactive software factory');
});

test('expired provider login returns a safe actionable failure without report retries', async () => {
  promptImplementation = async () => {
    eventHandler?.({
      type: 'message_end',
      message: {
        role: 'assistant',
        content: [],
        stopReason: 'error',
        errorMessage: 'OAuth refresh failed: refresh_token_expired private credential text',
        usage: emptyUsage
      }
    });
  };
  await using instance = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5'
  });
  const result = await instance.runOutcome(createTaskContext(), 'Work');
  expect(result).toMatchObject({
    outcome: 'failed',
    failureReason: 'provider_error',
    summary:
      'The model provider login has expired. Sign in again on the agent host before retrying.'
  });
  expect(promptCalls).toBe(1);
  expect(JSON.stringify(result)).not.toContain('private credential');
});

test.each(['root', 'child', 'report'])(
  'the CLI keeps run details and only prints the root answer with a %s agent',
  async (kind) => {
    const { mkdir, mkdtemp, readFile, readdir, rm, writeFile } = await import('node:fs/promises');
    const { fileURLToPath } = await import('node:url');
    const { join } = await import('node:path');
    const { runRunling } = await import('./runner.ts');
    const { step } = await import('./step.ts');
    const scratch = fileURLToPath(new URL('../../../../.context/', import.meta.url));
    // A clean checkout, including CI, does not have the gitignored scratch directory yet.
    await mkdir(scratch, { recursive: true });
    const directory = await mkdtemp(join(scratch, 'agent-cli-'));
    const runtime = fileURLToPath(new URL('./index.ts', import.meta.url));
    const workflow = join(directory, 'workflow.ts');
    const markdown = '## Answer once\n\n1. **First issue**\n2. Second issue';
    const reply = kind === 'root' ? markdown : 'Intermediate agent answer';
    const options = `{ name: 'Test agent', cwd: '.', model: 'anthropic/claude-opus-4-5' }`;
    await writeFile(
      workflow,
      `import { agent, createAgentSession, task, Type } from ${JSON.stringify(runtime)};
    export default ${
      kind === 'root'
        ? `agent(${options})`
        : `task({ name: 'Root', input: Type.String(), result: Type.String() }, async (ctx, input) => {
      ${kind === 'child' ? `await agent(${options})(ctx, input);` : `await using session = await createAgentSession(${options}); await session.runOutcome(ctx, input);`}
      return ${JSON.stringify(markdown)};
    })`
    };`
    );
    promptImplementation = async () => {
      eventHandler?.({
        type: 'tool_execution_start',
        toolName: 'gh',
        toolCallId: 'gh-call',
        args: {}
      });
      await step('Executing: gh api', async () => {});
      if (kind === 'report') await reportOutcome({ outcome: 'completed', summary: reply });
      else emitAssistantText(reply);
    };
    const print = vi.spyOn(console, 'log').mockImplementation(() => {});
    const cwd = vi.spyOn(process, 'cwd').mockReturnValue(directory);
    const stdout = process.stdout;
    const terminal = vi.spyOn(process, 'stdout', 'get').mockReturnValue(
      new Proxy(stdout, {
        get(target, key, receiver) {
          if (key === 'isTTY') return true;
          if (key === 'columns') return 80;
          return Reflect.get(target, key, receiver);
        }
      })
    );
    const exitCode = process.exitCode;
    try {
      await runRunling(workflow, 'List issues', { json: false, verbose: false });
      const output = print.mock.calls.map(([line]) => stripAnsi(line)).join('\n');
      const answers = print.mock.calls
        .map(([line]) => stripAnsi(line))
        .filter((line) => line.includes('Answer once'));
      expect(answers).toHaveLength(1);
      expect(answers[0]).toMatch(/^Answer once/);
      expect(stripAnsi(print.mock.calls.at(-1)![0])).toBe(answers[0]);
      expect(print.mock.calls.at(-2)![0]).toBe('');
      expect(output).toContain('Runling starting run');
      expect(output).toContain(kind === 'report' ? 'Root' : 'Test agent');
      expect(output).toContain('Token usage:');
      expect(output).toContain('Using gh');
      const agentId = output.match(/\[([^\]]+)\] Using gh/)?.[1];
      expect(agentId).toBeDefined();
      expect(output).toContain(`[${agentId}] Executing: gh api`);
      expect(output).toContain('Finished in');
      expect(output).toContain('journal: .runling/runs/');
      expect(output).toContain('First issue');
      expect(output).not.toContain('**');
      expect(output).not.toContain('Type a line');
      expect(output).not.toContain('Intermediate agent answer');
      const [journal] = await readdir(join(directory, '.runling/runs'));
      const records = (await readFile(join(directory, '.runling/runs', journal!), 'utf8'))
        .trim()
        .split('\n')
        .map((line) => JSON.parse(line));
      expect(records.at(-1)).toMatchObject({
        type: 'finished',
        status: 'completed',
        output: markdown
      });
      expect(records).toContainEqual(
        expect.objectContaining({
          type: 'event',
          event: expect.objectContaining({
            type: 'log',
            source: 'agent',
            message: expect.stringContaining(reply)
          })
        })
      );
      expect(disposed).toBe(true);
    } finally {
      process.exitCode = exitCode;
      terminal.mockRestore();
      cwd.mockRestore();
      print.mockRestore();
      await rm(directory, { recursive: true, force: true });
    }
  }
);

test('the agent task uses the session API and disposes it after forwarding text', async () => {
  const { agent } = await import('./agent-task.ts');
  const { runWorkflow } = await import('./runner.ts');
  promptImplementation = async () => {
    emitAssistantText('Task answer');
  };
  const worker = agent({
    name: 'Direct agent',
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5'
  });
  const replies: unknown[] = [];
  const result = await runWorkflow(worker, {
    input: 'Hello',
    onUpdate: (reply) => {
      replies.push(reply);
    }
  });
  expect(result).toMatchObject({ ok: true, output: 'Task answer' });
  expect(replies).toEqual(['Task answer']);
  expect(sessionOptions.customTools).toEqual([]);
  expect(disposed).toBe(true);
});

test('text mode delivers a natural response once without report tools or retries', async () => {
  const text = vi.fn();
  promptImplementation = async () => {
    emitAssistantText('Hello!');
  };
  const bot = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5',
    tools: [],
    output: 'text'
  });
  try {
    const result = await bot.run(createTaskContext(), 'Hi', { onText: text });
    expect(result).toMatchObject({ outcome: 'completed', summary: 'Hello!' });
    expect(text.mock.calls).toEqual([['Hello!']]);
    expect(promptCalls).toBe(1);
    expect(sessionOptions.customTools).toEqual([]);
    expect(sessionOptions.tools).toEqual([]);
    expect(resourceOptions.appendSystemPromptOverride(['Base'])).toEqual(['Base']);
  } finally {
    bot.dispose();
  }
});

test('renders assistant replies as terminal Markdown without changing the result or journal', async () => {
  const stdout = process.stdout;
  const stream = vi.spyOn(process, 'stdout', 'get').mockReturnValue(
    new Proxy(stdout, {
      get(target, key, receiver) {
        if (key === 'isTTY') return true;
        if (key === 'columns') return 60;
        return Reflect.get(target, key, receiver);
      }
    })
  );
  const print = vi.spyOn(console, 'log').mockImplementation(() => {});
  const markdown = '## Latest issues\n\n1. **Fix reconnect**';
  const events: RunlingEvent[] = [];
  promptImplementation = async () => emitAssistantText(markdown);
  try {
    const result = await observeRunlingEvents(
      (event) => events.push(event),
      () =>
        runAgent(createTaskContext(), 'List issues', {
          cwd: '/project',
          model: 'anthropic/claude-opus-4-5',
          output: 'text'
        })
    );
    expect(result.summary).toBe(markdown);
    const reply = print.mock.calls
      .map(([line]) => stripAnsi(line))
      .find((line) => line.includes('Latest issues'));
    expect(reply).toMatch(/^● \[[^\]]+\]\n/);
    expect(reply).toContain('Fix reconnect');
    expect(reply).not.toContain('##');
    expect(reply).not.toContain('**');
    expect(events).toContainEqual(
      expect.objectContaining({
        type: 'log',
        source: 'agent',
        message: expect.stringContaining(markdown)
      })
    );
  } finally {
    print.mockRestore();
    stream.mockRestore();
  }
});

test('text mode keeps progress delivery but returns only the final response', async () => {
  const text = vi.fn();
  promptImplementation = async () => {
    emitAssistantText('Checking the page.');
    emitAssistantText('Here is the answer.');
  };
  const bot = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5',
    output: 'text'
  });
  try {
    expect((await bot.run(createTaskContext(), 'Look it up', { onText: text })).summary).toBe(
      'Here is the answer.'
    );
    expect(text.mock.calls).toEqual([['Checking the page.'], ['Here is the answer.']]);
  } finally {
    bot.dispose();
  }
});

test.each(['initial', 'report-repair'])(
  'provider exhaustion in %s does not trigger another report request',
  async (phase) => {
    promptImplementation = async () => {
      if (phase === 'report-repair' && promptCalls === 1) return;
      eventHandler?.({
        type: 'auto_retry_start',
        attempt: 2,
        maxAttempts: 2,
        delayMs: 4000,
        errorMessage: 'secret upstream request'
      });
      eventHandler?.({
        type: 'auto_retry_end',
        success: false,
        attempt: 2,
        finalError: 'secret upstream request'
      });
    };
    const result = await runAgent(createTaskContext(), 'Investigate', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5'
    });
    expect(result.outcome).toBe('failed');
    expect(result.failureReason).toBe('provider_error');
    expect(promptCalls).toBe(phase === 'initial' ? 1 : 2);
    expect(JSON.stringify(result)).not.toContain('secret');
  }
);

test('tool events exclude raw arguments and results', async () => {
  const events: RunlingEvent[] = [];
  promptImplementation = async () => {
    for (const isError of [true, false]) {
      eventHandler?.({
        type: 'tool_execution_start',
        toolCallId: 'edit',
        toolName: 'edit',
        args: { path: 'private/path' }
      });
      eventHandler?.({
        type: 'tool_execution_end',
        toolCallId: 'edit',
        toolName: 'edit',
        isError,
        result: { content: [{ type: 'text', text: 'secret output' }] }
      });
    }
    await reportOutcome({ outcome: 'completed', summary: 'Done' });
  };
  await observeRunlingEvents(
    (event) => events.push(event),
    () =>
      runAgent(createTaskContext(), 'Investigate', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      })
  );
  const tools = events.filter((event) => event.type === 'agent.tool');
  expect(tools.map(({ toolName, operation, phase }) => ({ toolName, operation, phase }))).toEqual([
    { toolName: 'edit', operation: 'edit', phase: 'started' },
    { toolName: 'edit', operation: 'edit', phase: 'failed' },
    { toolName: 'edit', operation: 'edit', phase: 'started' },
    { toolName: 'edit', operation: 'edit', phase: 'succeeded' }
  ]);
  expect(JSON.stringify(tools)).not.toMatch(/private|secret/);
});

test('a provider retry stays one interaction and does not deliver the failed text', async () => {
  const events: RunlingEvent[] = [];
  promptImplementation = async () => {
    eventHandler?.({ type: 'agent_start' });
    eventHandler?.({
      type: 'message_end',
      message: {
        role: 'assistant',
        content: [{ type: 'text', text: 'Partial answer' }],
        stopReason: 'error',
        errorMessage: 'WebSocket error',
        usage: emptyUsage
      }
    });
    eventHandler?.({
      type: 'auto_retry_start',
      attempt: 1,
      maxAttempts: 3,
      delayMs: 2000,
      errorMessage: 'WebSocket error'
    });
    // Pi starts its agent loop again for the retry.
    eventHandler?.({ type: 'agent_start' });
    eventHandler?.({ type: 'auto_retry_end', success: true, attempt: 1 });
    eventHandler?.({
      type: 'message_end',
      message: {
        role: 'assistant',
        content: [{ type: 'text', text: 'Full answer' }],
        stopReason: 'stop',
        usage: emptyUsage
      }
    });
  };
  const onText = vi.fn();
  await using instance = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5',
    output: 'text'
  });
  const result = await observeRunlingEvents(
    (event) => events.push(event),
    () => instance.runOutcome(createTaskContext(), 'Investigate', { onText })
  );
  expect(result).toMatchObject({ outcome: 'completed', summary: 'Full answer' });
  expect(onText.mock.calls).toEqual([['Full answer']]);
  expect(events.filter((event) => event.type === 'agent.started')).toHaveLength(1);
});

test('a successful provider retry can still produce a valid report', async () => {
  promptImplementation = async () => {
    eventHandler?.({
      type: 'message_end',
      message: { role: 'assistant', content: [], stopReason: 'error', errorMessage: 'temporary' }
    });
    eventHandler?.({
      type: 'auto_retry_start',
      attempt: 1,
      maxAttempts: 2,
      delayMs: 2000,
      errorMessage: 'temporary'
    });
    await reportOutcome({ outcome: 'completed', summary: 'Recovered' });
    eventHandler?.({ type: 'auto_retry_end', success: true, attempt: 1 });
  };
  expect(
    (
      await runAgent(createTaskContext(), 'Investigate', {
        cwd: '/project',
        model: 'anthropic/claude-opus-4-5'
      })
    ).outcome
  ).toBe('completed');
  expect(promptCalls).toBe(1);
});

test.each(['error', 'aborted', 'empty'])(
  'text mode does not report success after %s',
  async (reason) => {
    promptImplementation = async () => {
      eventHandler?.({
        type: 'message_end',
        message: {
          role: 'assistant',
          content: reason === 'empty' ? [] : [{ type: 'text', text: 'Partial' }],
          stopReason: reason === 'empty' ? 'stop' : reason,
          errorMessage: reason === 'empty' ? undefined : 'Provider failed'
        }
      });
    };
    const result = await runAgent(createTaskContext(), 'Hi', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5',
      output: 'text'
    });
    expect(result.outcome).toBe('failed');
    expect(promptCalls).toBe(1);
  }
);

test.each([
  { name: 'one terminating tool', results: [true], completed: true },
  { name: 'all tools terminate', results: [true, true], completed: true },
  { name: 'mixed batch', results: [true, false], completed: false },
  { name: 'mixed batch in reverse order', results: [false, true], completed: false },
  { name: 'failed terminating tool', results: [true], toolError: true, completed: false },
  { name: 'provider failure', results: [true], providerError: true, completed: false },
  { name: 'later empty turn', results: [true], laterTurn: true, completed: false },
  { name: 'nested result alone', results: [true], nested: true, completed: false }
])('text mode handles $name without allowing arbitrary silence', async (scenario) => {
  promptImplementation = async () => {
    eventHandler?.({ type: 'turn_start' });
    eventHandler?.({
      type: 'message_end',
      message: { role: 'assistant', content: [], usage: emptyUsage, stopReason: 'toolUse' }
    });
    scenario.results.forEach((terminate, index) =>
      eventHandler?.({
        type: 'tool_execution_end',
        toolCallId: `tool-${index}`,
        toolName: 'finish',
        result: { terminate },
        isError: scenario.toolError ?? false,
        ...(scenario.nested ? { parentToolCallId: 'parent' } : {})
      })
    );
    if (scenario.laterTurn) eventHandler?.({ type: 'turn_start' });
    if (scenario.providerError)
      eventHandler?.({
        type: 'message_end',
        message: {
          role: 'assistant',
          content: [],
          usage: emptyUsage,
          stopReason: 'error',
          errorMessage: 'Provider failed'
        }
      });
  };
  const result = await runAgent(createTaskContext(), 'Complete through a tool', {
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5',
    output: 'text'
  });
  expect(result.outcome).toBe(scenario.completed ? 'completed' : 'failed');
  if (scenario.completed) expect(result.summary).toBe('');
});

test('tool completion does not carry into the next interaction of a session', async () => {
  const bot = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5',
    output: 'text'
  });
  try {
    promptImplementation = async () => {
      eventHandler?.({
        type: 'tool_execution_end',
        toolCallId: 'finish',
        toolName: 'finish',
        result: { terminate: true },
        isError: false
      });
    };
    expect((await bot.runOutcome(createTaskContext(), 'Finish')).outcome).toBe('completed');
    promptImplementation = async () => {};
    expect((await bot.runOutcome(createTaskContext(), 'Another request')).outcome).toBe('failed');
  } finally {
    bot.dispose();
  }
});

test.each(['stop', 'error', 'aborted'])(
  'allowEmptyText accepts silence but preserves provider failures: %s',
  async (stopReason) => {
    promptImplementation = async () => {
      eventHandler?.({
        type: 'message_end',
        message: {
          role: 'assistant',
          content: [],
          stopReason,
          errorMessage: stopReason === 'stop' ? undefined : 'Provider failed'
        }
      });
    };
    const result = await runAgent(createTaskContext(), 'Update the reaction silently', {
      cwd: '/project',
      model: 'anthropic/claude-opus-4-5',
      output: 'text',
      allowEmptyText: true
    });
    expect(result.outcome).toBe(stopReason === 'stop' ? 'completed' : 'failed');
    if (stopReason === 'stop') expect(result.summary).toBe('');
    expect(promptCalls).toBe(1);
  }
);

test('text mode retains usage accounting and resets response state between turns', async () => {
  const ctx = createTaskContext();
  const bot = await createAgentSession({
    cwd: '/project',
    model: 'anthropic/claude-opus-4-5',
    output: 'text'
  });
  promptImplementation = async () => {
    eventHandler?.({
      type: 'message_end',
      message: {
        role: 'assistant',
        content: [{ type: 'text', text: 'First' }],
        usage: { input: 10, output: 2, cacheRead: 0, cacheWrite: 0 },
        stopReason: 'stop'
      }
    });
  };
  try {
    const first = await bot.run(ctx, 'Hi');
    expect(first.usage.input).toBe(10);
    expect(ctx.usage.input).toBe(10);

    promptImplementation = async () => {};
    expect((await bot.runOutcome(ctx, 'Again')).outcome).toBe('failed');
    expect(ctx.usage.input).toBe(10);
  } finally {
    bot.dispose();
  }
});
