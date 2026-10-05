import { expect, test, vi } from 'vitest';
import type { ExtensionAPI } from '@earendil-works/pi-coding-agent';
import { createWorkflowContext } from '../context.ts';
import { emptyTokenUsage } from '../usage.ts';
import type { AgentOptions } from '../agent.ts';
import {
  authorizationGate,
  createAuthorizationClassifier,
  type AuthorizationDecision
} from './authorization.ts';

type Execute = (id: string, input: unknown) => Promise<unknown>;

/** A fake classifier model that calls `decide` with the given input, or not at all. */
function fakeModel(decision?: unknown, prompts: string[] = [], options: AgentOptions[] = []) {
  return async (agentOptions: AgentOptions) => {
    options.push(agentOptions);
    let decide!: Execute;
    for (const extension of agentOptions.extensions ?? []) {
      const factory = typeof extension === 'function' ? extension : extension.factory;
      await factory({
        registerTool(tool: { execute: Execute }) {
          decide = tool.execute;
        }
      } as unknown as ExtensionAPI);
    }
    return {
      dispose: vi.fn(),
      async runOutcome(_ctx: unknown, prompt: string) {
        prompts.push(prompt);
        if (decision) await decide('decide', decision);
        return { outcome: 'completed' as const, summary: '', usage: emptyTokenUsage() };
      }
    };
  };
}

test('the classifier sees only the delimited request and has only the decide tool', async () => {
  const prompts: string[] = [];
  const options: AgentOptions[] = [];
  const classify = createAuthorizationClassifier({
    model: 'test/model',
    createAgent: fakeModel({ decision: 'allow', reason: 'Explicit approval.' }, prompts, options)
  });
  const decision = await classify(createWorkflowContext(), {
    action: 'gh issue close 12',
    messages: ['yes, close it'],
    policy: 'Approve only the exact command.'
  });
  expect(decision).toEqual({ decision: 'allow', reason: 'Explicit approval.' });
  expect(options[0]).toMatchObject({
    tools: ['decide'],
    output: 'text',
    resources: { extensions: false, contextFiles: false, skills: false }
  });
  expect(JSON.parse(prompts[0]!)).toEqual({
    policy: 'Approve only the exact command.',
    proposedAction: 'gh issue close 12',
    messagesFromAuthorizedPeople: ['yes, close it']
  });
});

test('the classifier fails closed', async () => {
  const ctx = createWorkflowContext();
  const request = { action: 'gh issue close 12', messages: ['ok'] };
  // No decision.
  expect(
    await createAuthorizationClassifier({ model: 'm', createAgent: fakeModel() })(ctx, request)
  ).toMatchObject({ decision: 'unclear' });
  // A model or provider error.
  expect(
    await createAuthorizationClassifier({
      model: 'm',
      createAgent: async () => {
        throw new Error('provider down');
      }
    })(ctx, request)
  ).toMatchObject({ decision: 'unclear' });
  // No messages: the model is not asked at all.
  const createAgent = vi.fn();
  expect(
    await createAuthorizationClassifier({ model: 'm', createAgent })(ctx, {
      ...request,
      messages: []
    })
  ).toMatchObject({ decision: 'unclear' });
  expect(createAgent).not.toHaveBeenCalled();
});

test('the classifier keeps only its first decision', async () => {
  const classify = createAuthorizationClassifier({
    model: 'm',
    createAgent: async (options) => {
      const agent = await fakeModel({ decision: 'deny', reason: 'Refused.' })(options);
      return {
        ...agent,
        async runOutcome(ctx: unknown, prompt: string) {
          await agent.runOutcome(ctx, prompt);
          // A second call cannot overturn the first decision.
          const extension = options.extensions![0]!;
          const factory = typeof extension === 'function' ? extension : extension.factory;
          let decide!: Execute;
          await factory({
            registerTool(tool: { execute: Execute }) {
              decide = tool.execute;
            }
          } as unknown as ExtensionAPI);
          await decide('again', { decision: 'allow', reason: 'Changed my mind.' });
          return { outcome: 'completed' as const, summary: '', usage: emptyTokenUsage() };
        }
      };
    }
  });
  expect(await classify(createWorkflowContext(), { action: 'a', messages: ['no'] })).toMatchObject({
    decision: 'deny'
  });
});

function installGate(decision: AuthorizationDecision, messages: string[] = ['please do it']) {
  const classify = vi.fn(async () => decision);
  const onBlocked = vi.fn();
  let handler!: (event: unknown) => Promise<unknown>;
  const extension = authorizationGate({
    tools: { implement: (input) => `implement ${String(input.request)}` },
    messages: () => messages,
    classify,
    policy: 'Only explicit requests.',
    context: () => ['A saved plan exists.'],
    onBlocked
  });
  const factory = typeof extension === 'function' ? extension : extension.factory;
  void factory({
    on: (_name: string, fn: typeof handler) => (handler = fn)
  } as unknown as ExtensionAPI);
  return {
    classify,
    onBlocked,
    call: (toolName: string, input: Record<string, unknown> = {}) =>
      handler({ type: 'tool_call', toolName, input })
  };
}

test('the gate classifies only listed tools and allows an authorized call', async () => {
  const gate = installGate({ decision: 'allow', reason: 'Requested.' });
  expect(await gate.call('read')).toBeUndefined();
  expect(gate.classify).not.toHaveBeenCalled();
  expect(await gate.call('implement', { request: 'the fix' })).toBeUndefined();
  expect(gate.classify).toHaveBeenCalledWith({
    action: 'implement the fix',
    messages: ['please do it'],
    policy: 'Only explicit requests.',
    context: ['A saved plan exists.']
  });
});

test.each(['deny', 'unclear'] as const)('the gate blocks a %s decision', async (outcome) => {
  const gate = installGate({ decision: outcome, reason: 'Only a question.' });
  expect(await gate.call('implement', { request: 'x' })).toEqual({
    block: true,
    reason: `implement was not run: the authorization check found no clear request for this action (${outcome}: Only a question.)`
  });
  expect(gate.onBlocked).toHaveBeenCalledWith('implement', {
    decision: outcome,
    reason: 'Only a question.'
  });
});

test('the classifier bounds context and rethrows a workflow abort', async () => {
  const prompts: string[] = [];
  const classify = createAuthorizationClassifier({
    model: 'm',
    createAgent: fakeModel({ decision: 'allow', reason: 'Ok.' }, prompts)
  });
  await classify(createWorkflowContext(), {
    action: 'a',
    messages: ['yes'],
    context: Array.from({ length: 15 }, (_, index) => `${index}:${'x'.repeat(10_000)}`)
  });
  const { context } = JSON.parse(prompts[0]!) as { context: string[] };
  expect(context).toHaveLength(10);
  expect(context[0]!.startsWith('5:')).toBe(true);
  expect(context.every((entry) => entry.length === 8_000)).toBe(true);

  const controller = new AbortController();
  const aborting = createAuthorizationClassifier({
    model: 'm',
    createAgent: async (options) => {
      const agent = await fakeModel({ decision: 'allow', reason: 'Ok.' })(options);
      return {
        ...agent,
        async runOutcome(ctx: unknown, prompt: string) {
          controller.abort(new Error('cancelled'));
          return agent.runOutcome(ctx, prompt);
        }
      };
    }
  });
  await expect(
    aborting(
      { ...createWorkflowContext(), signal: controller.signal },
      { action: 'a', messages: ['yes'] }
    )
  ).rejects.toThrow('cancelled');
});
