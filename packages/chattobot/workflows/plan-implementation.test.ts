import { expect, test, vi } from 'vitest';
import { createWorkflowContext } from 'runling';
import type {
  AgentExtensionAPI,
  AgentOptions,
  AuthorizationDecision,
  AuthorizationRequest
} from 'runling/agents';
import type { InvestigationPlans } from './plan.ts';

const { interact } = vi.hoisted(() => ({ interact: vi.fn() }));
vi.mock('runling/agents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling/agents')>()),
  runAgentConversation: interact
}));
// A fake investigation saves a plan, as a completed implementation investigation does.
vi.mock('./investigate.ts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./investigate.ts')>()),
  investigationExtension:
    (
      _ctx: unknown,
      _settings: unknown,
      _announce: unknown,
      _tasks: unknown,
      plans: InvestigationPlans
    ) =>
    (pi: AgentExtensionAPI) =>
      pi.registerTool({
        name: 'fakeInvestigate',
        execute: async () => {
          plans.set('investigation-1', {
            goal: 'Keep the newest message visible when the keyboard opens',
            steps: [{ files: ['EventList.svelte'], change: 'Observe the viewport height' }],
            acceptanceCriteria: ['The newest message stays visible'],
            checks: ['Focused tests'],
            openQuestions: [],
            baseCommit: 'abc'
          });
        }
      } as never)
}));
import { conversation } from './chat.ts';

type Gate = (event: unknown) => Promise<unknown>;

/** Run every tool_call gate in order, as Pi does: the first block wins. */
const gated = async (gates: Gate[], input: Record<string, unknown>) => {
  for (const gate of gates) {
    const result = await gate({ type: 'tool_call', toolName: 'implementChatto', input });
    if (result) return result;
  }
  return undefined;
};

test.each([
  {
    name: 'starts a saved plan that a maintainer asked to implement',
    request: 'Yeah, make me a PR for this please.',
    decision: 'allow' as const
  },
  {
    name: 'does not start a plan that a maintainer only asked to plan',
    request: 'Please plan how to fix this.',
    decision: 'unclear' as const
  }
])('a plan notification $name', async ({ request, decision }) => {
  const tools = new Map<string, { execute(id: string, input: never): Promise<unknown> }>();
  const gates: Gate[] = [];
  const requests: AuthorizationRequest[] = [];
  const replies: string[] = [];
  interact.mockImplementationOnce(async (_ctx, _agent, _prompt, options) => {
    options.onBusy(true);
    await options.prepareMessage(request, 'user');
    await tools.get('fakeInvestigate')!.execute('call', {} as never);
    await options.prepareMessage(
      '{"type":"task.completed","taskId":"investigation-1"}',
      'notification'
    );
    // Only a saved plan of this conversation passes the maintainer gate on a notification turn.
    for (const input of [
      { investigationId: 'unknown' },
      { investigationId: 'investigation-1', resumeArtifactId: 'implementation-abcdef' },
      {}
    ])
      expect(await gated(gates.slice(0, 1), input)).toMatchObject({
        block: true,
        reason: expect.stringContaining('background notification')
      });
    const result = await gated(gates, { request: 'Fix it', investigationId: 'investigation-1' });
    if (decision === 'allow') expect(result).toBeUndefined();
    else expect(result).toMatchObject({ block: true });
    // The authorization check read the maintainer's own request and the saved plan.
    expect(requests).toHaveLength(1);
    expect(requests[0]!.messages).toEqual([request]);
    expect(requests[0]!.context?.join('\n')).toContain('Keep the newest message visible');
    return 'done';
  });
  await conversation(
    {
      ...createWorkflowContext(),
      emit: async (text) => {
        replies.push(text);
      }
    },
    request,
    {
      createAgent: async (options: AgentOptions) => {
        for (const extension of options.extensions ?? []) {
          const factory = typeof extension === 'function' ? extension : extension.factory;
          await factory({
            on(name: string, handler: Gate) {
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
      classifier: async (_ctx, authorization): Promise<AuthorizationDecision> => {
        requests.push(authorization);
        return { decision, reason: 'Checked.' };
      },
      investigation: { directory: '/unused' },
      implementation: { directory: '/unused', repository: 'example/chatto' },
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
      requester: () => 'maintainer',
      currentMessageId: () => undefined,
      isAddressed: () => true,
      announce: async () => {}
    }
  );
  // Notification turns post no host refusal.
  expect(replies).toEqual([]);
});
