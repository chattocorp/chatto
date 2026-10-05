import { afterEach, expect, test, vi } from 'vitest';
import { createChannel, createWorkflowContext, emptyTokenUsage } from '../index.ts';
import type { AgentExtension, AgentExtensionAPI } from '../agent.ts';
import {
  approvalDecisionExtension,
  createApprovalQueue,
  toolApprovalGate,
  type ApprovalAction,
  type ApprovalDecision
} from './approvals.ts';
import { observeAgentTasks, type AgentTaskUpdate } from './tasks.ts';
import { runAgentConversation } from './conversation.ts';
import type { WorkflowContext } from '../context.ts';

afterEach(() => vi.useRealTimers());
const action: ApprovalAction = {
  action: 'publish',
  details: { repository: 'example/repo', revision: 'tree-1' }
};
const allow: ApprovalDecision = { decision: 'allow', reason: 'Within delegated scope.' };

test('decisions are single use, bound to one owner, and proposals are snapshots', async () => {
  const signal = new AbortController().signal;
  const owner = createApprovalQueue({ signal });
  const other = createApprovalQueue({ signal });
  const input = structuredClone(action);
  const result = owner.request(input, {
    signal,
    notify: (notice) => {
      notice.details.revision = 'mutated';
    }
  });
  input.details.revision = 'also-mutated';
  const [request] = owner.list();
  request!.details.revision = 'changed snapshot';
  expect(owner.list()[0]!.details.revision).toBe('tree-1');
  expect(other.decide(request!.id, allow)).toBe(false);
  expect(owner.decide('invented-id', allow)).toBe(false);
  expect(owner.decide(request!.id, allow)).toBe(true);
  expect(owner.decide(request!.id, allow)).toBe(false);
  expect(await result).toEqual(allow);
  expect(owner.list()).toEqual([]);
  owner.dispose();
  other.dispose();
});

test.each(['owner', 'child', 'dispose'] as const)(
  'a %s stop denies the waiting action',
  async (stop) => {
    const ownerSignal = new AbortController();
    const childSignal = new AbortController();
    const queue = createApprovalQueue({ signal: ownerSignal.signal });
    const result = queue.request(action, { signal: childSignal.signal, notify() {} });
    const id = queue.list()[0]!.id;
    if (stop === 'owner') ownerSignal.abort();
    else if (stop === 'child') childSignal.abort();
    else queue.dispose();
    expect((await result).decision).toBe('deny');
    expect(queue.decide(id, allow)).toBe(false);
    expect(queue.list()).toEqual([]);
    queue.dispose();
  }
);

test('expiry, notice failure, bounds, and a full queue fail closed', async () => {
  vi.useFakeTimers();
  const signal = new AbortController().signal;
  const queue = createApprovalQueue({ signal, timeoutMs: 50, maxPending: 1 });
  const notify = vi.fn();
  const waiting = queue.request(action, { signal, notify });
  const id = queue.list()[0]!.id;
  expect((await queue.request(action, { signal, notify })).decision).toBe('deny');
  await vi.advanceTimersByTimeAsync(51);
  expect((await waiting).decision).toBe('deny');
  expect(queue.decide(id, allow)).toBe(false);
  expect(
    (
      await queue.request(action, {
        signal,
        notify: () => {
          throw new Error('offline');
        }
      })
    ).decision
  ).toBe('deny');
  expect(
    (
      await queue.request(
        { action: 'x', details: { text: 'x'.repeat(16_000) } },
        { signal, notify }
      )
    ).decision
  ).toBe('deny');
  queue.dispose();
});

/** Install only the extension hook under test, without a provider call. */
async function gate(
  request: (action: ApprovalAction) => Promise<ApprovalDecision>,
  signal = new AbortController().signal
) {
  let hook!: (event: { toolName: string; input: Record<string, unknown> }) => Promise<unknown>;
  const extension = toolApprovalGate({
    signal,
    tools: {
      publish: (input) => ({ action: 'publish', details: input as ApprovalAction['details'] })
    },
    request
  });
  const factory = typeof extension === 'function' ? extension : extension.factory;
  await factory({
    on(_name: string, handler: typeof hook) {
      hook = handler;
    }
  } as unknown as AgentExtensionAPI);
  return hook;
}

test('a gated call waits for its owner; other calls retain existing policy', async () => {
  const decision = Promise.withResolvers<ApprovalDecision>();
  const request = vi.fn(() => decision.promise);
  const hook = await gate(request);
  expect(await hook({ toolName: 'read', input: {} })).toBeUndefined();
  expect(request).not.toHaveBeenCalled();
  const complete = vi.fn();
  const pending = hook({ toolName: 'publish', input: { revision: 'tree-1' } }).then(complete);
  await Promise.resolve();
  expect(complete).not.toHaveBeenCalled();
  decision.resolve(allow);
  await pending;
  expect(complete).toHaveBeenCalledWith(undefined);
});

test.each(['deny', 'failure', 'mutation', 'cancel'] as const)(
  'a %s blocks execution',
  async (scenario) => {
    const signal = new AbortController();
    const event = { toolName: 'publish', input: { revision: 'tree-1' } };
    const hook = await gate(async () => {
      if (scenario === 'failure') throw new Error('provider unavailable');
      if (scenario === 'mutation') event.input.revision = 'tree-2';
      if (scenario === 'cancel') signal.abort();
      return scenario === 'deny' ? { decision: 'deny', reason: 'Outside scope.' } : allow;
    }, signal.signal);
    expect(await hook(event)).toMatchObject({ block: true });
  }
);

test('a waiting child wakes its owner, which remains responsive to user input', async () => {
  const root = createWorkflowContext();
  const inbox = createChannel<string>({ signal: root.signal });
  const ctx = { ...root, inbox };
  const queue = createApprovalQueue({ signal: ctx.signal });
  const tasks = observeAgentTasks(ctx, { notifyActivity: false });
  let decide!: (
    id: string,
    input: { id: string; decision: 'allow'; reason: string }
  ) => Promise<unknown>;
  const extension: AgentExtension = approvalDecisionExtension(queue);
  const factory = typeof extension === 'function' ? extension : extension.factory;
  await factory({
    registerTool(tool: { execute: typeof decide }) {
      decide = tool.execute;
    }
  } as unknown as AgentExtensionAPI);
  const executed = vi.fn();
  let notified = false;
  const prompts: string[] = [];
  const owner = {
    async runOutcome(_ctx: unknown, prompt: string) {
      prompts.push(prompt);
      const [request] = queue.list();
      // Leave it pending until the user gives a further instruction.
      if (prompt === 'Proceed' && request)
        await decide('decision', { id: request.id, decision: 'allow', reason: 'Within scope.' });
      return { outcome: 'completed' as const, summary: '', usage: emptyTokenUsage() };
    },
    steer: async () => false
  };
  const child = ctx.spawn(async (child: WorkflowContext<string, AgentTaskUpdate>) => {
    const decision = await queue.request(action, {
      signal: child.signal,
      notify: async (request) => {
        await child.emit({
          type: 'notice',
          text: 'Owner decision required.',
          data: { approvalId: request.id }
        } satisfies AgentTaskUpdate);
        notified = true;
      }
    });
    child.signal.throwIfAborted();
    if (decision.decision === 'allow') executed();
    return decision.decision;
  });
  tasks.observe('Worker', child);
  const conversation = runAgentConversation(ctx, owner, 'Start', {
    notifications: tasks.notifications,
    keepAlive: () => tasks.active,
    timeout: 0.03
  });
  await vi.waitFor(() => expect(notified).toBe(true));
  await vi.waitFor(() => expect(prompts.some((text) => text.includes('task.notice'))).toBe(true));
  expect(executed).not.toHaveBeenCalled();
  await inbox.send('Proceed');
  await conversation;
  expect(executed).toHaveBeenCalledOnce();
  expect(await child.result).toBe('allow');
  queue.dispose();
  await tasks.dispose();
});
