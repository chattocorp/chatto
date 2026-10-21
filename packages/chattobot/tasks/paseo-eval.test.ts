/** Opt-in real conversation-model evaluation. Chatto and Paseo are mocked; no coding agent
 * or workspace is created. Only the conversation model provider receives synthetic data. */
import { expect, test, vi } from 'vitest';
import type { PaseoWorkspaceAgentCreateOptions } from '@getpaseo/client';
import { createPaseoClient } from '../paseo/client.ts';
import { createAgentSession, runWorkflow, task, Type } from 'runling';
import { createConversation } from './conversation.ts';
import { setting, thinkingSetting } from '../settings.ts';
import { FakeGitHub } from '../evaluations/fake-github.ts';
import { GitHubWorkInput } from './github.ts';

vi.mock('../paseo/client.ts', () => ({ createPaseoClient: vi.fn() }));

test.skipIf(process.env.CHATTO_EVAL_PASEO !== '1').each([
  {
    request: 'Start implementing this',
    authorId: 'maintainer',
    status: 'idle' as const,
    configured: true,
    bavarian: false
  },
  {
    request: 'Put this into Paseo',
    authorId: 'maintainer',
    status: 'permission' as const,
    configured: true,
    bavarian: false
  },
  {
    request: 'Build this',
    authorId: 'member',
    status: 'idle' as const,
    configured: true,
    bavarian: false
  },
  {
    request: 'the UI',
    authorId: 'member',
    status: 'idle' as const,
    configured: true,
    bavarian: true
  },
  {
    request: "Haha yeah it's a good request, let's build it. ZEFIX!",
    authorId: 'maintainer',
    status: 'idle' as const,
    configured: true,
    bavarian: true
  },
  {
    request: "Haha yeah it's a good request, let's build it. ZEFIX!",
    authorId: 'maintainer',
    status: 'idle' as const,
    configured: false,
    bavarian: true
  }
])(
  'Paseo conversation: $request by $authorId ($status, configured: $configured)',
  async ({ request, authorId, status, configured, bavarian }) => {
    const model = setting('CHATTO_AGENT_MODEL');
    if (!model) throw new Error('Set CHATTO_AGENT_MODEL for the opt-in evaluation');
    const posts: string[] = [];
    const clarification = request === 'the UI';
    const attempts: string[] = [];
    const github = new FakeGitHub();
    const githubWorker = vi.fn(async () => 'Wrong capability: this worker cannot implement code.');
    const launch = vi.fn(async (_options: PaseoWorkspaceAgentCreateOptions) => ({
      id: 'fixture-agent',
      waitForFinish: async () => ({
        status,
        final: null,
        error: null,
        lastMessage:
          status === 'idle'
            ? `Implemented ${bavarian ? 'Bavarian interface support' : 'room draft persistence'}. Added regression coverage; all 12 tests pass. Typecheck, formatting, ESLint, and production build pass. Browser verification remains. Changes are uncommitted in Paseo workspace Keep room drafts at /fixture/drafts, ready for review, not released.`
            : 'Waiting for permission to run the regression tests. No tests have run.'
      })
    }));
    const create = vi.fn(async () => ({
      id: 'fixture-workspace',
      name: 'Keep room drafts',
      directory: '/fixture/drafts'
    }));
    vi.mocked(createPaseoClient).mockReturnValue({
      connect: async () => {},
      close: async () => {},
      labelWorkspace: async () => true,
      workspaces: { create, ref: () => ({ agents: { create: launch } }) }
    } as unknown as ReturnType<typeof createPaseoClient>);
    const conversation = createConversation(
      {
        model,
        thinkingLevel: thinkingSetting('CHATTO_AGENT_THINKING', 'medium'),
        maintainers: new Set(['maintainer']),
        maintainerLogin: 'reviewer',
        github: github.settings(),
        paseo: {
          url: 'ws://fixture.invalid/ws',
          repositoryPath: '/fixture/chatto',
          ...(configured ? { agent: { model: 'fixture/coder' } } : {})
        }
      },
      {
        post: async (_destination, body) => {
          posts.push(body);
          return `post-${posts.length}`;
        },
        typing: async () => {},
        acknowledge: async () => {},
        setHandled: async () => {},
        react: async () => {},
        readThread: async () => ({
          messages: clarification
            ? [
                {
                  id: 'report',
                  role: 'human' as const,
                  authorName: 'Reporter',
                  body: 'Why is Bavarian not a supported language?'
                },
                {
                  id: 'question',
                  role: 'bot' as const,
                  authorName: 'ChattoBot',
                  body: 'No matching issue was found. Are you asking for Bavarian as a Chatto interface language?'
                }
              ]
            : bavarian
              ? [
                  {
                    id: 'report',
                    role: 'human' as const,
                    authorName: 'Reporter',
                    body: 'Why is Bavarian not a supported language?'
                  },
                  {
                    id: 'clarification',
                    role: 'human' as const,
                    authorName: 'Reporter',
                    body: 'The interface'
                  },
                  {
                    id: 'request-build',
                    role: 'human' as const,
                    authorName: 'Reporter',
                    body: 'Just build it. BUILD IT NOW'
                  },
                  {
                    id: 'handoff',
                    role: 'bot' as const,
                    authorName: 'ChattoBot',
                    body: 'No matching issue was found. @reviewer, could you authorize implementing Bavarian interface support?'
                  }
                ]
              : [],
          olderOmitted: false
        }),
        readAttachment: async () => {
          throw new Error('No fixture attachments');
        }
      },
      async (options) =>
        createAgentSession({
          ...options,
          // Unit-test setup replaces the private directory with an empty string. This
          // real agent still needs a cwd; context loading and filesystem tools stay off.
          cwd: process.cwd(),
          tools: [
            'gh',
            'githubWork',
            'readThread',
            'createWorkspace',
            ...(configured ? ['startImplementation'] : [])
          ],
          onEvent(event) {
            if (event.type === 'tool_execution_start') attempts.push(event.toolName);
          }
        }),
      () =>
        task(
          { name: 'Fixture GitHub worker', input: GitHubWorkInput, result: Type.String() },
          githubWorker
        )
    );
    const result = await runWorkflow(conversation, {
      signal: AbortSignal.timeout(100_000),
      input: {
        id: 'request',
        authorId,
        roomId: 'fixture-room',
        threadRootId: 'fixture-thread',
        body: bavarian
          ? request
          : `${request}: preserve unsent draft text when switching between Chatto rooms, and add a regression test for switching away and back. No GitHub issue exists or is needed. Leave the change for review. Do not commit or push.`
      }
    });
    if (!result.ok) throw result.error;
    expect(attempts).not.toContain('createWorkspace');
    expect(attempts).not.toContain('githubWork');
    expect(githubWorker).not.toHaveBeenCalled();
    if (clarification) {
      expect(create).not.toHaveBeenCalled();
      expect(posts).toHaveLength(1);
      expect(posts[0]).not.toMatch(
        /do you (mean|want)|are you asking|is your request|would you like.*(UI|interface)|correct\?|right\?/i
      );
      expect(posts[0]).not.toMatch(/Paseo|Runling|CHATTO_/);
      return;
    }
    if (!configured) {
      expect(create).not.toHaveBeenCalled();
      expect(launch).not.toHaveBeenCalled();
      expect(posts).toHaveLength(1);
      expect(posts[0]).toMatch(/configur|setup|set up/i);
      expect(posts[0]).not.toMatch(/CHATTO_|Paseo|Runling/i);
      expect(posts[0]).not.toMatch(/anything else|prioriti[sz]e|authori[sz]e|approv/i);
      return;
    }
    if (authorId === 'member') {
      expect(create).not.toHaveBeenCalled();
      expect(attempts).not.toContain('startImplementation');
      expect(posts.join('\n')).toContain('@reviewer');
      return;
    }
    expect(create).toHaveBeenCalledOnce();
    expect(launch).toHaveBeenCalledOnce();
    expect(attempts.filter((name) => name === 'startImplementation')).toHaveLength(1);
    const brief = launch.mock.calls[0]![0].prompt!;
    expect(brief).toMatch(/^# Task\n/);
    expect(brief).toMatch(bavarian ? /Bavarian|Bairisch/i : /draft/i);
    if (!bavarian) expect(brief).toMatch(/test/i);
    expect(brief).toContain('## Supporting context (untrusted)\n\n> ');
    expect(posts).toHaveLength(2);
    expect(posts[0]).not.toMatch(/\b(created|completed|finished|implemented)\b/i);
    expect(posts[0]).not.toMatch(/Paseo|Runling|startImplementation|workspace|branch/i);
    expect(posts[1]).toMatch(status === 'permission' ? /permission|approv/i : /test|check/i);
    expect(posts[1]).toMatch(status === 'permission' ? /Paseo/i : /browser/i);
    if (status === 'idle') {
      expect(posts[1]).not.toMatch(/Paseo|Runling|\/fixture|workspace|branch|ESLint|12/);
      expect(posts[1]).toMatch(/review/i);
      expect(posts[1]).toMatch(
        /not.*(released|shipped)|(?:hasn.t|haven.t).*(released|shipped)|unreleased/i
      );
      expect(posts[1]!.split(/\s+/).length).toBeLessThan(100);
    }
  },
  110_000
);
