import { expect, test, vi } from 'vitest';
import { createWorkflowContext } from 'runling';
import type { AgentExtensionAPI, AgentOptions } from 'runling/agents';

const { interact } = vi.hoisted(() => ({ interact: vi.fn() }));
vi.mock('runling/agents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling/agents')>()),
  runAgentConversation: interact
}));
// A fake investigation accepts delegation without starting an LLM or subprocess.
vi.mock('./investigate.ts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./investigate.ts')>()),
  investigationExtension:
    (
      _ctx: unknown,
      _settings: unknown,
      announce: (text: string, signal: AbortSignal) => Promise<void>
    ) =>
    (pi: AgentExtensionAPI) =>
      pi.registerTool({
        name: 'fakeInvestigate',
        execute: async () => announce('Investigation started.', new AbortController().signal)
      } as never)
}));
import { conversation } from './chat.ts';

test.each(['accepted', 'refused'])(
  '%s delegation posts one host-owned reply and permits later replies',
  async (mode) => {
    const tools = new Map<string, { execute(id: string, input: never): Promise<unknown> }>();
    let gate: ((event: unknown) => Promise<unknown>) | undefined;
    const replies: string[] = [];
    const createAgent = async (options: AgentOptions) => {
      expect(options.textDelivery).toBe('final');
      for (const extension of options.extensions ?? []) {
        const factory = typeof extension === 'function' ? extension : extension.factory;
        await factory({
          on(name: string, handler: (event: unknown) => Promise<unknown>) {
            if (name === 'tool_call') gate = handler;
          },
          registerTool(tool: { name: string }) {
            tools.set(tool.name, tool as never);
          }
        } as unknown as AgentExtensionAPI);
      }
      return { runOutcome: vi.fn(), steer: async () => false, dispose() {} };
    };
    interact.mockImplementationOnce(async (ctx, _agent, _prompt, options) => {
      options.onBusy(true);
      if (mode === 'accepted') {
        await tools.get('fakeInvestigate')!.execute('call', {} as never);
        // The announcement owns this turn; the supervisor's second version is suppressed.
        await ctx.emit("I'm working on the fix now.");
        expect(replies).toEqual(['Investigation started.']);
      } else {
        // A notification never authorizes work. The gate blocks it without a chat message.
        await options.prepareMessage('Investigation complete', 'notification');
        for (let i = 0; i < 2; i++)
          expect(
            await gate!({ type: 'tool_call', toolName: 'implementChatto', input: {} })
          ).toMatchObject({
            block: true,
            reason: expect.stringContaining('background notification')
          });
        await ctx.emit('The investigation finished.');
        expect(replies).toEqual(['The investigation finished.']);
      }
      options.onBusy(false);
      options.onBusy(true);
      await ctx.emit('Here is the answer to your next question.');
      return 'done';
    });
    await conversation(
      {
        ...createWorkflowContext(),
        emit: async (text) => {
          replies.push(text);
        }
      },
      'Assess this',
      {
        createAgent,
        model: 'test/model',
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
          message: { id: 'message', author_id: 'human', body: 'Assess this' }
        },
        readThread: async () => ({ messages: [], olderOmitted: false }),
        maintainers: ['human'],
        onBusy() {},
        setReplyContext() {},
        requester: () => 'human',
        announce: async (text) => {
          replies.push(text);
        }
      }
    );
    expect(replies).toHaveLength(2);
    expect(replies[1]).toBe('Here is the answer to your next question.');
  }
);
