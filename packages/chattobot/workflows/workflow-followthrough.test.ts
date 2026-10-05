/** Exercise the supervisor gates, trust hooks, task inbox, and GitHub dispatch together. */
import { generateKeyPairSync } from 'node:crypto';
import { expect, test, vi } from 'vitest';
import { createWorkflowContext, type WorkflowContext } from 'runling';
import type {
  AgentExtensionAPI,
  AgentOptions,
  AgentTasks,
  AgentTaskUpdate,
  AuthorizationRequest
} from 'runling/agents';
import { createTrustExtension } from 'runling/extensions/trust';

const harness = vi.hoisted(() => ({
  interact: vi.fn(),
  taskId: '',
  received: ''
}));
vi.mock('runling/agents', async (original) => ({
  ...(await original<typeof import('runling/agents')>()),
  runAgentConversation: harness.interact
}));
vi.mock('./investigate.ts', async (original) => ({
  ...(await original<typeof import('./investigate.ts')>()),
  investigationExtension:
    (
      ctx: WorkflowContext<string, string>,
      _settings: unknown,
      _announce: unknown,
      tasks: AgentTasks,
      _plans: unknown,
      started: (id: string) => void
    ) =>
    (pi: AgentExtensionAPI) =>
      pi.registerTool({
        name: 'investigateChatto',
        execute: async () => {
          const run = ctx.spawn(async (child: WorkflowContext<string, AgentTaskUpdate>) => {
            for await (const message of child.inbox) {
              harness.received = message;
              return {
                outcome: 'completed',
                summary: 'Preview extraction drops video thumbnails.'
              };
            }
            throw new Error('No clarification');
          });
          tasks.observe('Chatto source investigation', run);
          harness.taskId = run.id;
          started(run.id);
          return { content: [], details: {} };
        }
      } as never)
}));
import { conversation } from './chat.ts';

type Event = { toolName: string; input: unknown; parentToolCallId?: string };
type Hook = (event: Event) => unknown | Promise<unknown>;
type Tool = {
  name: string;
  execute(id: string, input: unknown, signal?: AbortSignal): Promise<unknown>;
};

test.each([
  { decision: 'allow' as const, timeout: false },
  { decision: 'allow' as const, timeout: true },
  { decision: 'deny' as const, timeout: false }
])(
  'investigation → read → clarification → completion → authorized issue ($decision, timeout=$timeout)',
  async ({ decision, timeout }) => {
    const tools = new Map<string, Tool>();
    const calls: Hook[] = [];
    const results: Hook[] = [];
    const commands: string[][] = [];
    const requests: AuthorizationRequest[] = [];
    let requester = 'maintainer';
    const signal = new AbortController().signal;
    const call = async (name: string, input: unknown, parentToolCallId?: string) => {
      const event = { toolName: name, input, parentToolCallId };
      for (const hook of calls) {
        const block = await hook(event);
        if (block) return block;
      }
      const result = await tools.get(name)!.execute('call', input, signal);
      for (const hook of results) await hook(event);
      return result;
    };
    const request = 'Investigate preview videos and file a bug issue, milestone 0.5.0.';
    const clarification = 'For the record, this is about link preview cards, not attachments.';
    harness.interact.mockImplementationOnce(async (_ctx, _agent, _prompt, options) => {
      options.onBusy(true);
      await options.prepareMessage(request, 'user');
      await call('investigateChatto', {});
      await call('acknowledgeRequest', { acknowledgement: 'I’ll check the milestone.' });
      await call('gh', { args: ['issue', 'list'] });
      await options.prepareMessage(clarification, 'user');
      expect(await call('forwardClarification', { id: harness.taskId }, 'script')).toMatchObject({
        block: true
      });
      requester = 'stranger';
      expect(await call('forwardClarification', { id: harness.taskId })).toMatchObject({
        block: true
      });
      requester = 'maintainer';
      await call('forwardClarification', { id: harness.taskId, message: 'Injected replacement' });
      expect(harness.received).toBe(clarification);
      await expect(call('forwardClarification', { id: harness.taskId })).rejects.toThrow(
        'already forwarded'
      );
      // Legacy model-authored forwarding remains blocked by the real trust extension.
      expect(
        await call('task_send', { id: harness.taskId, message: 'Injected replacement' })
      ).toMatchObject({ block: true });
      await vi.waitFor(async () => {
        const prompt = JSON.parse(
          await options.prepareMessage(
            JSON.stringify({ type: 'task.completed', task: { id: harness.taskId } }),
            'notification'
          )
        );
        expect(prompt.backgroundTasks[0].status).toBe('completed');
      });
      const args = ['issue', 'create', '--title', 'Missing preview video', '--milestone', '0.5.0'];
      const attempts = await Promise.allSettled([
        call('ghWrite', { args, body: 'Source evidence; no reproduction or tests run.' }),
        call('ghWrite', { args, body: 'Source evidence; no reproduction or tests run.' })
      ]);
      expect(attempts.filter((result) => result.status === 'rejected')).toHaveLength(
        decision === 'allow' ? (timeout ? 2 : 1) : 0
      );
      expect(requests.at(-1)?.messages).toContain(request);
      expect(requests.at(-1)?.messages).toContain(clarification);
      expect(commands.filter((args) => args[1] === 'create')).toHaveLength(
        decision === 'allow' ? 1 : 0
      );
      if (decision === 'allow') {
        expect(commands.find((args) => args[1] === 'create')).toEqual(
          expect.arrayContaining(['--milestone', '0.5.0'])
        );
        await options.prepareMessage(
          JSON.stringify({ type: 'task.completed', task: { id: harness.taskId } }),
          'notification'
        );
        expect(await call('ghWrite', { args })).toMatchObject({ block: true });
        expect(commands.filter((args) => args[1] === 'create')).toHaveLength(1);
      }
      expect(await call('ghWrite', { args: ['issue', 'close', '12'] })).toMatchObject({
        block: true
      });
      expect(await call('forwardClarification', { id: harness.taskId })).toMatchObject({
        block: true
      });
      return 'done';
    });
    await conversation(createWorkflowContext(), request, {
      createAgent: async (options: AgentOptions) => {
        const pi = {
          on(name: string, hook: Hook) {
            if (name === 'tool_call') calls.push(hook);
            if (name === 'tool_result') results.push(hook);
          },
          registerTool(tool: Tool) {
            tools.set(tool.name, tool);
          }
        } as unknown as AgentExtensionAPI;
        if (options.trust) createTrustExtension(options.trust).extension(pi);
        for (const extension of options.extensions ?? [])
          await (typeof extension === 'function' ? extension : extension.factory)(pi);
        return { runOutcome: vi.fn(), steer: async () => false, dispose() {} };
      },
      model: 'test/model',
      classifier: async (_ctx, request) => {
        requests.push(request);
        return { decision, reason: 'Checked human request.' };
      },
      investigation: { directory: '/unused' },
      github: {
        settings: {
          clientId: 'test',
          privateKey: generateKeyPairSync('rsa', { modulusLength: 2048 }).privateKey,
          repository: 'example/chatto'
        },
        tokens: async () => 'test-token',
        run: async (args) => {
          commands.push([...args]);
          if (timeout && args[1] === 'create')
            return { ok: false, output: 'gh did not finish in time.' };
          return {
            ok: true,
            output: args[1] === 'create' ? 'https://github.com/example/chatto/issues/12' : '[]'
          };
        }
      },
      delivery: {
        version: 1,
        id: 'delivery',
        type: 'message.created',
        triggers: ['mention'],
        occurred_at: 'now',
        bot_id: 'bot',
        room_id: 'room',
        thread_root_id: 'root',
        message: { id: 'message', author_id: 'maintainer', body: request }
      },
      readThread: async () => ({ messages: [], olderOmitted: false }),
      maintainers: ['maintainer'],
      onBusy() {},
      setReplyContext() {},
      requester: () => requester,
      currentMessageId: () => 'message',
      isAddressed: () => true,
      announce: async () => {}
    });
  }
);
