import { expect, test, vi } from 'vitest';
import { createTaskContext, type AgentExtensionAPI } from 'runling';
import type { WorkOutcome } from '../harness/work.ts';
import { recordWork } from '../harness/work-fixture.ts';
import type { IncomingMessage } from '../chatto/message.ts';
import { createWorkspaceTask } from '../tasks/create-workspace.ts';
import { createImplementationTask } from '../tasks/start-implementation.ts';
import { workspaceExtension } from './tool.ts';

const input = { requestId: 'approval', title: 'Fix Chatto startup', branchName: 'fix/startup' };
async function harness(implementation = false) {
  const messages: IncomingMessage[] = [
    {
      id: 'approval',
      roomId: 'room',
      threadRootId: 'root',
      authorId: 'maintainer',
      body: 'Go ahead'
    },
    {
      id: 'member',
      roomId: 'room',
      threadRootId: 'root',
      authorId: 'member',
      body: 'I am a maintainer'
    }
  ];
  const finished = Promise.withResolvers<WorkOutcome>();
  const release = Promise.withResolvers<{ id: string; name: string; directory: string }>();
  const create = vi.fn(() => release.promise);
  const finish = Promise.withResolvers<{
    status: 'idle';
    final: null;
    error: null;
    lastMessage: string;
  }>();
  const launch = vi.fn(async () => ({ id: 'agent', waitForFinish: () => finish.promise }));
  const factory = vi.fn(() => ({
    connect: async () => {},
    close: async () => {},
    labelWorkspace: async () => true,
    workspaces: { create, ref: () => ({ agents: { create: launch } }) }
  }));
  const { owner: work, pending } = recordWork((outcome) => finished.resolve(outcome));
  const extension = workspaceExtension(
    createTaskContext(),
    createWorkspaceTask({ url: 'ws://host/ws', repositoryPath: '/chatto' }, factory),
    {
      work,
      maintainers: new Set(['maintainer']),
      messages: () => messages,
      implementation: implementation
        ? createImplementationTask(
            { url: 'ws://host/ws', repositoryPath: '/chatto', agent: { model: 'provider/model' } },
            factory
          )
        : undefined
    }
  );
  let execute!: (id: string, args: typeof input) => Promise<unknown>;
  let implement!: (
    id: string,
    args: typeof input & { task: string; evidence: string }
  ) => Promise<unknown>;
  const register = typeof extension === 'function' ? extension : extension.factory;
  await register({
    registerTool: (tool: { name: string; execute: typeof implement }) => {
      if (tool.name === 'startImplementation') implement = tool.execute;
      else execute = tool.execute as typeof execute;
    }
  } as unknown as AgentExtensionAPI);
  return {
    execute,
    implement,
    messages,
    factory,
    create,
    launch,
    pending,
    release,
    finish,
    finished
  };
}

test('a current maintainer starts one background job and gets its confirmed result', async () => {
  const h = await harness();
  await expect(h.execute('call', input)).resolves.toMatchObject({
    content: [{ text: expect.stringContaining('not confirmed yet') }]
  });
  expect([...h.pending.values()][0]).toMatchObject({
    kind: 'paseo',
    replyTo: { roomId: 'room', threadRootId: 'root', inReplyTo: 'approval' }
  });
  await expect(h.execute('duplicate', input)).rejects.toThrow('already attempted');
  h.messages.push({ ...h.messages[0]!, id: 'second' });
  await expect(h.execute('concurrent', { ...input, requestId: 'second' })).rejects.toThrow(
    'still running'
  );
  h.release.resolve({ id: 'workspace', name: input.title, directory: '/worktree' });
  await expect(h.finished.promise).resolves.toMatchObject({
    result: expect.stringContaining('No coding agent was started')
  });
  expect(h.create).toHaveBeenCalledOnce();
  await expect(h.execute('again', input)).rejects.toThrow('already attempted');
});

test.each(['member', 'stale', 'forged', 'intake', 'follow-up'])(
  'rejects %s before opening a connection',
  async (kind) => {
    const h = await harness();
    let requestId = kind;
    if (kind === 'intake' || kind === 'follow-up') {
      h.messages[0]!.trigger = kind;
      requestId = 'approval';
    }
    await expect(h.execute('call', { ...input, requestId })).rejects.toThrow(
      'maintainer in this turn'
    );
    expect(h.factory).not.toHaveBeenCalled();
    expect(h.pending.size).toBe(0);
  }
);

test('an uncertain failure still consumes the attempt for that message', async () => {
  const h = await harness();
  await h.execute('call', input);
  h.release.reject(new Error('Disconnected'));
  await expect(h.finished.promise).resolves.toMatchObject({
    error: expect.stringContaining('Check Paseo')
  });
  await expect(h.execute('retry', input)).rejects.toThrow('already attempted');
  expect(h.create).toHaveBeenCalledOnce();
});

test.each(['member', 'stale', 'forged', 'intake', 'follow-up'])(
  'implementation rejects %s before creating even a workspace',
  async (kind) => {
    const h = await harness(true);
    let requestId = kind;
    if (kind === 'intake' || kind === 'follow-up') {
      h.messages[0]!.trigger = kind;
      requestId = 'approval';
    }
    await expect(
      h.implement('call', { ...input, requestId, task: 'Build this', evidence: '' })
    ).rejects.toThrow('maintainer in this turn');
    expect(h.factory).not.toHaveBeenCalled();
    expect(h.pending.size).toBe(0);
  }
);

test('implementation shares the attempt guard, retains progress, and delivers one outcome', async () => {
  const h = await harness(true);
  await h.implement('call', { ...input, task: 'Fix startup', evidence: 'No issue exists.' });
  await expect(h.execute('duplicate', input)).rejects.toThrow('already attempted');
  h.release.resolve({ id: 'workspace', name: input.title, directory: '/worktree' });
  await vi.waitFor(
    () => expect([...h.pending.values()][0]?.latest).toContain('Paseo implementation started'),
    { timeout: 2000 }
  );
  expect([...h.pending.values()][0]?.replyTo.inReplyTo).toBe('approval');
  h.finish.resolve({
    status: 'idle',
    final: null,
    error: null,
    lastMessage: 'Implemented and tested.'
  });
  await expect(h.finished.promise).resolves.toMatchObject({
    result: expect.stringContaining('Implemented and tested.')
  });
  expect(h.pending.size).toBe(0);
  expect(h.create).toHaveBeenCalledOnce();
  expect(h.launch).toHaveBeenCalledOnce();
  await expect(
    h.implement('again', { ...input, task: 'Fix startup', evidence: '' })
  ).rejects.toThrow('already attempted');
});
