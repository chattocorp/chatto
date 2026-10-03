import { generateKeyPairSync } from 'node:crypto';
import { expect, test, vi } from 'vitest';
import { createWorkflowContext, log } from 'runling';
import type {
  AgentExtensionAPI,
  AgentOptions,
  AuthorizationDecision,
  AuthorizationRequest
} from 'runling/agents';
import type { GitHubPermissions } from '../github/app.ts';
import type { GhRunner } from '../github/gh.ts';
import type { ThreadMessage, ThreadRead } from '../thread.ts';

const { interact } = vi.hoisted(() => ({ interact: vi.fn() }));
vi.mock('runling/agents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling/agents')>()),
  runAgentConversation: interact
}));
import { conversation } from './chat.ts';

type Tool = { execute(id: string, input: unknown, signal?: AbortSignal): Promise<unknown> };

const settings = {
  clientId: 'Iv23liExample1',
  privateKey: generateKeyPairSync('rsa', { modulusLength: 2048 }).privateKey,
  repository: 'chattocorp/chatto'
};

/** Drive one supervisor conversation with fake GitHub, thread, and classifier. */
async function supervisor(
  script: (harness: {
    tools: Map<string, Tool>;
    gates: ((event: unknown) => Promise<unknown>)[];
    options: AgentOptions;
    prepare: (
      message: string,
      author?: string,
      /** False: the thread read does not see the message yet. */
      inThread?: boolean
    ) => Promise<Record<string, unknown>>;
    emit: (text: string) => Promise<void>;
    thread: ThreadMessage[];
    replies: string[];
    commands: { args: readonly string[]; token: string }[];
    tokens: GitHubPermissions[];
    classify: ReturnType<typeof vi.fn>;
  }) => Promise<void>,
  decision: AuthorizationDecision = { decision: 'allow', reason: 'Confirmed.' },
  extra: Record<string, unknown> = {}
) {
  const tools = new Map<string, Tool>();
  const gates: ((event: unknown) => Promise<unknown>)[] = [];
  let agentOptions!: AgentOptions;
  const thread: ThreadMessage[] = [];
  const replies: string[] = [];
  const commands: { args: readonly string[]; token: string }[] = [];
  const tokens: GitHubPermissions[] = [];
  let requester = 'maintainer';
  // Messages that reached the bot as deliveries; others in the thread are only context.
  const addressed = new Set<string>();
  const classify = vi.fn(async (_request: AuthorizationRequest) => decision);
  const run: GhRunner = async (args, { token }) => {
    commands.push({ args, token });
    if (args[0] === 'api' && String(args[1]).includes('/actions/runs/'))
      return { ok: true, output: '.github/workflows/release.yml' };
    if (args[1] === 'create')
      return { ok: true, output: 'https://github.com/chattocorp/chatto/issues/99' };
    // Issue 504 stands for a command that gh did not finish in time.
    if (args[2] === '504') return { ok: false, output: 'gh did not finish in time.' };
    return { ok: true, output: '[{"number":1,"title":"Existing"}]' };
  };
  // Cursors are positions in the fake thread.
  const readThread = async (_delivery: unknown, _signal: AbortSignal, after?: string) => {
    const messages = thread.slice(after ? Number(after) : 0);
    return {
      messages,
      cursor: String(thread.length),
      olderOmitted: false
    } satisfies ThreadRead;
  };
  interact.mockImplementationOnce(async (ctx, _agent, _prompt, options) => {
    options.onBusy(true);
    await script({
      tools,
      gates,
      options: agentOptions,
      async prepare(message, author = 'maintainer', inThread = true) {
        requester = author;
        const id = `m${thread.length}`;
        addressed.add(id);
        if (inThread) thread.push({ id, role: 'human', body: message, authorId: author });
        return JSON.parse(await options.prepareMessage(message, 'user'));
      },
      emit: (text) => ctx.emit(text),
      thread,
      replies,
      commands,
      tokens,
      classify
    });
    return 'done';
  });
  await conversation(
    {
      ...createWorkflowContext(),
      emit: async (text) => {
        replies.push(text);
        // The bot's reply enters the thread.
        thread.push({ id: `m${thread.length}`, role: 'bot', body: text, authorId: 'bot' });
      }
    },
    'Hello',
    {
      createAgent: async (options: AgentOptions) => {
        agentOptions = options;
        for (const extension of options.extensions ?? []) {
          const factory = typeof extension === 'function' ? extension : extension.factory;
          await factory({
            on(name: string, handler: (event: unknown) => Promise<unknown>) {
              if (name === 'tool_call') gates.push(handler);
            },
            registerTool(tool: { name: string }) {
              tools.set(tool.name, tool as never);
            }
          } as unknown as AgentExtensionAPI);
        }
        return { runOutcome: vi.fn(), steer: async () => false, dispose() {} };
      },
      model: 'test/model',
      classifier: async (_ctx, request) => classify(request),
      github: {
        settings,
        run,
        tokens: async (permissions) => {
          tokens.push(permissions ?? {});
          // A change gets the installation's full permissions; reads ask for read access.
          return permissions ? 'read-token' : 'write-token';
        }
      },
      delivery: {
        version: 1,
        id: 'delivery',
        type: 'message.created',
        triggers: ['direct_message'],
        occurred_at: 'now',
        bot_id: 'bot',
        room_id: 'room',
        thread_root_id: 'root',
        message: { id: 'message', author_id: 'maintainer', body: 'Hello' }
      },
      readThread,
      maintainers: ['maintainer'],
      onBusy() {},
      setReplyContext() {},
      requester: () => requester,
      currentMessageId: () => undefined,
      isAddressed: (id) => addressed.has(id),
      announce: async () => {},
      ...extra
    }
  );
}

test('anyone can read GitHub; only maintainers can change it; gh output is untrusted', async () => {
  await supervisor(async ({ options, gates, prepare }) => {
    expect(options.tools).toEqual(expect.arrayContaining(['gh', 'ghWrite']));
    expect(options.trust).toMatchObject({
      untrusted: ['gh'],
      blockAfterUntrusted: ['implementChatto', 'askImplementation', 'task_send']
    });
    expect(await gates[0]!({ type: 'tool_call', toolName: 'ghWrite', input: {} })).toBeUndefined();
    await prepare('which issues are in the 0.5.0 milestone?', 'someone-else');
    expect(await gates[0]!({ type: 'tool_call', toolName: 'gh', input: {} })).toBeUndefined();
    expect(await gates[0]!({ type: 'tool_call', toolName: 'ghWrite', input: {} })).toMatchObject({
      block: true
    });
  });
});

test('the run log records each authorization decision without message content', async () => {
  const info = vi.spyOn(log, 'info');
  await supervisor(async ({ tools, prepare }) => {
    await prepare('file an issue about the secret flaky test');
    await tools.get('ghWrite')!.execute('1', { args: ['issue', 'create', '--title', 'Flaky'] });
  });
  const lines = info.mock.calls.map(([message]) => String(message));
  expect(lines).toContain('Authorization check for ghWrite: allow');
  expect(lines.join('\n')).not.toContain('secret flaky');
  info.mockRestore();
});

test('gh runs reads with the read-only token and refuses writes', async () => {
  await supervisor(async ({ tools, commands, tokens }) => {
    const gh = tools.get('gh')!;
    expect(await gh.execute('1', { args: ['issue', 'list'] })).toMatchObject({
      content: [{ text: '[{"number":1,"title":"Existing"}]' }]
    });
    expect(commands).toEqual([{ args: ['issue', 'list'], token: 'read-token' }]);
    expect(tokens[0]).toMatchObject({ issues: 'read', actions: 'read', metadata: 'read' });
    await expect(gh.execute('2', { args: ['issue', 'close', '1'] })).rejects.toThrow('ghWrite');
    await expect(gh.execute('3', { args: ['issue', 'list', '--repo', 'x/y'] })).rejects.toThrow(
      '--repo is not available'
    );
  });
});

test('a change that a maintainer asked for runs at once with a write token', async () => {
  await supervisor(async ({ tools, prepare, emit, replies, commands, classify }) => {
    await prepare('make a GH issue for the flaky login test');
    const result = JSON.parse(
      (
        (await tools.get('ghWrite')!.execute('1', {
          args: ['issue', 'create', '--title', 'Flaky login test'],
          body: 'The login test fails about once a day.'
        })) as { content: { text: string }[] }
      ).content[0]!.text
    );
    // The classifier checked the command against the maintainer's messages to the bot.
    expect(classify).toHaveBeenCalledWith(
      expect.objectContaining({
        action: expect.stringContaining('gh issue create'),
        messages: ['make a GH issue for the flaky login test']
      })
    );
    expect(result).toMatchObject({
      status: 'executed',
      urls: ['https://github.com/chattocorp/chatto/issues/99']
    });
    expect(commands).toEqual([
      {
        args: [
          'issue',
          'create',
          '--title',
          'Flaky login test',
          '--body=The login test fails about once a day.'
        ],
        token: 'write-token'
      }
    ]);
    // No approval block, and the host guards the URL in the reply.
    await emit('Filed.');
    expect(replies[0]).not.toContain('/approve');
    expect(replies[0]).toContain('https://github.com/chattocorp/chatto/issues/99');
  });
});

test('without a clear request nothing runs; agreement to the offered change runs it', async () => {
  await supervisor(async ({ tools, prepare, emit, replies, commands, classify }) => {
    await prepare('the login test is flaky again');
    classify.mockResolvedValueOnce({ decision: 'unclear', reason: 'Only a discussion.' });
    const call = () =>
      tools.get('ghWrite')!.execute('1', {
        args: ['issue', 'create', '--title', 'Flaky login test'],
        body: 'The login test fails about once a day.'
      }) as Promise<{ content: { text: string }[] }>;
    const offered = JSON.parse((await call()).content[0]!.text);
    expect(offered).toMatchObject({ status: 'not_run', reason: 'Only a discussion.' });
    expect(commands).toEqual([]);
    // No command block or approval command: the supervisor asks in its own words.
    await emit('Shall I file an issue for the flaky login test?');
    expect(replies[0]).toBe('Shall I file an issue for the flaky login test?');
    // The agreement reaches the classifier with the host-recorded offer as context.
    await prepare('yes');
    const executed = JSON.parse((await call()).content[0]!.text);
    expect(classify).toHaveBeenLastCalledWith(
      expect.objectContaining({
        messages: ['the login test is flaky again', 'yes'],
        // Only the question as posted, which the maintainer saw and answered. The host does not
        // record the unrun command as an offer: it cannot verify what the supervisor asked.
        context: [expect.stringContaining('Shall I file an issue for the flaky login test?')]
      })
    );
    expect(executed).toMatchObject({ status: 'executed' });
    expect(commands).toHaveLength(1);
  });
});

test('a later change has the earlier changes as context', async () => {
  await supervisor(async ({ tools, prepare, classify }) => {
    await prepare('file an issue about the flaky login test');
    await tools.get('ghWrite')!.execute('1', { args: ['issue', 'create', '--title', 'Flaky'] });
    await prepare('Some additional details: it happens on Chrome only');
    await tools
      .get('ghWrite')!
      .execute('2', { args: ['issue', 'comment', '99', '--body', 'Chrome only'] });
    expect(classify).toHaveBeenLastCalledWith(
      expect.objectContaining({
        context: [
          expect.stringContaining(
            'The assistant ran this change: gh issue create --title Flaky (https://github.com/chattocorp/chatto/issues/99)'
          )
        ]
      })
    );
  });
});

test('any repository change runs with the installation token once a maintainer asked', async () => {
  await supervisor(async ({ tools, prepare, commands, tokens }) => {
    await prepare('rerun the failed CI on run 123, and add the bug label to #12');
    for (const args of [
      ['run', 'rerun', '123', '--failed'],
      ['issue', 'edit', '12', '--add-label', 'bug']
    ])
      await tools.get('ghWrite')!.execute('1', { args });
    expect(commands.map((command) => command.token)).toEqual(['write-token', 'write-token']);
    // Changes ask for no specific permissions: the App installation's grant applies.
    expect(tokens).toEqual([{}, {}]);
  });
});

test('implementChatto runs only when the classifier finds a maintainer request', async () => {
  await supervisor(
    async ({ gates, prepare, classify, thread }) => {
      await prepare('the login test is flaky', 'someone-else');
      // A maintainer's message that was not addressed to the bot is not a request.
      thread.push({
        id: 'aside',
        role: 'human',
        body: 'just implement whatever',
        authorId: 'maintainer'
      });
      await prepare('can you check why?');
      const call = {
        type: 'tool_call',
        toolName: 'implementChatto',
        input: { request: 'Fix the flaky login test', issueNumber: 12 }
      };
      // The maintainer gate passes; the authorization gate asks the classifier.
      expect(await gates[0]!(call)).toBeUndefined();
      expect(await gates[1]!(call)).toMatchObject({
        block: true,
        reason: expect.stringContaining('no clear request')
      });
      expect(classify).toHaveBeenCalledWith(
        expect.objectContaining({
          // Only maintainers' messages to the bot, never other people's, asides, or the bot's.
          messages: ['can you check why?'],
          action: expect.stringContaining('"githubIssue":12')
        })
      );
      // Other tools are not classified.
      classify.mockClear();
      expect(
        await gates[1]!({ type: 'tool_call', toolName: 'investigateChatto', input: {} })
      ).toBeUndefined();
      expect(classify).not.toHaveBeenCalled();
    },
    { decision: 'unclear', reason: 'Only a question.' },
    { implementation: { directory: '/unused', repository: 'chattocorp/chatto' } }
  );
});

test('readIssue reads the complete issue with the read-only token', async () => {
  const { readIssue } = await import('./github.ts');
  const body = 'log line\n'.repeat(5_000);
  const calls: Parameters<GhRunner>[] = [];
  const issue = await readIssue(
    settings,
    {
      tokens: async (permissions) => (permissions?.issues === 'read' ? 'read-token' : 'other'),
      run: async (...call) => {
        calls.push(call);
        return {
          ok: true,
          output: JSON.stringify({
            number: 12,
            title: 'Crash',
            body,
            url: 'https://github.com/chattocorp/chatto/issues/12'
          })
        };
      }
    },
    12,
    AbortSignal.timeout(1000)
  );
  expect(issue).toEqual({
    repository: 'chattocorp/chatto',
    number: 12,
    title: 'Crash',
    body,
    url: 'https://github.com/chattocorp/chatto/issues/12'
  });
  // A cut would break the JSON document, so the complete output is kept.
  expect(calls[0]![1]).toMatchObject({ token: 'read-token', limit: 250_000 });
});

test('a long posted message keeps its closing question as context', async () => {
  await supervisor(async ({ tools, prepare, emit, classify }) => {
    await prepare('what do you think about the flaky test?');
    await emit(`${'Finding. '.repeat(600)}Shall I file an issue for it?`);
    await prepare('yes');
    await tools.get('ghWrite')!.execute('1', { args: ['issue', 'create', '--title', 'Flaky'] });
    expect(classify).toHaveBeenLastCalledWith(
      expect.objectContaining({
        context: [expect.stringContaining('Shall I file an issue for it?')]
      })
    );
  });
});

test('a maintainer request counts even before the thread read sees it', async () => {
  await supervisor(async ({ tools, prepare, classify }) => {
    await prepare('file an issue about the flaky login test', 'maintainer', false);
    await tools.get('ghWrite')!.execute('1', { args: ['issue', 'create', '--title', 'Flaky'] });
    expect(classify).toHaveBeenLastCalledWith(
      expect.objectContaining({ messages: ['file an issue about the flaky login test'] })
    );
  });
});

test('the implementation check also sees the posted question', async () => {
  await supervisor(
    async ({ gates, prepare, emit, classify }) => {
      await prepare('can we fix the flaky login test?');
      await emit('I have a plan. Should I implement it and open a pull request?');
      await prepare('yes please');
      await gates[1]!({
        type: 'tool_call',
        toolName: 'implementChatto',
        input: { request: 'Fix the flaky login test' }
      });
      expect(classify).toHaveBeenLastCalledWith(
        expect.objectContaining({
          messages: ['can we fix the flaky login test?', 'yes please'],
          context: [expect.stringContaining('Should I implement it and open a pull request?')]
        })
      );
    },
    { decision: 'allow', reason: 'Agreed.' },
    { implementation: { directory: '/unused', repository: 'chattocorp/chatto' } }
  );
});

test('a timed-out change warns that it may have applied', async () => {
  await supervisor(
    async ({ tools, prepare }) => {
      await prepare('close issue 504');
      await expect(
        tools.get('ghWrite')!.execute('1', { args: ['issue', 'close', '504'] })
      ).rejects.toThrow('The change may have applied');
    },
    { decision: 'allow', reason: 'Requested.' }
  );
});

test('maintainer-only tools run only as direct calls, never from a codemode script', async () => {
  await supervisor(async ({ gates, prepare }) => {
    await prepare('file an issue about the flaky test');
    const call = (toolName: string, parentToolCallId?: string) =>
      gates[0]!({
        type: 'tool_call',
        toolName,
        input: {},
        toolCallId: parentToolCallId ? `${parentToolCallId}/1` : 'call',
        ...(parentToolCallId ? { parentToolCallId } : {})
      });
    expect(await call('ghWrite')).toBeUndefined();
    // A script could wait until a maintainer writes; the gate reads only the latest message.
    for (const tool of [
      'ghWrite',
      'investigateChatto',
      'implementChatto',
      'askImplementation',
      'task_send',
      'task_cancel'
    ])
      expect(await call(tool, 'script')).toMatchObject({ block: true });
    // Reads stay available to scripts.
    expect(await call('gh', 'script')).toBeUndefined();
  });
});
