import { expect, test, vi } from 'vitest';
import { createTaskContext } from 'runling';
import { createWorkspaceTask, type WorkspaceClient } from './create-workspace.ts';

const input = { title: 'Fix issue 42', branchName: 'fix/issue-42' };
function harness() {
  const create = vi.fn<WorkspaceClient['workspaces']['create']>().mockResolvedValue({
    id: 'workspace-1',
    name: input.title,
    directory: '/worktrees/issue-42'
  });
  const connect = vi.fn(async () => {});
  const close = vi.fn(async () => {});
  const labelWorkspace = vi.fn(async (_id: string) => true);
  const factory = vi.fn(() => ({ connect, close, labelWorkspace, workspaces: { create } }));
  const task = createWorkspaceTask(
    { url: 'wss://paseo.test/ws', password: 'private', repositoryPath: '/remote/chatto' },
    factory
  );
  return { task, factory, connect, close, create, labelWorkspace };
}

test('creates only an isolated worktree on the configured host and closes the connection', async () => {
  const h = harness();
  expect(await h.task(createTaskContext(), input)).toEqual({
    id: 'workspace-1',
    name: input.title,
    directory: '/worktrees/issue-42',
    branchName: input.branchName,
    labelApplied: true
  });
  expect(h.factory).toHaveBeenCalledWith(
    expect.objectContaining({
      reconnect: { enabled: false },
      url: 'wss://paseo.test/ws',
      password: 'private'
    })
  );
  expect(h.create).toHaveBeenCalledExactlyOnceWith({
    source: {
      kind: 'worktree',
      cwd: '/remote/chatto',
      action: 'branch-off',
      refName: 'origin/main',
      branchName: input.branchName
    },
    title: input.title
  });
  expect(h.close).toHaveBeenCalledOnce();
  expect(h.labelWorkspace).toHaveBeenCalledExactlyOnceWith('workspace-1');
});

test('label failure preserves a confirmed workspace without retrying creation', async () => {
  const h = harness();
  h.labelWorkspace.mockRejectedValue(new Error('Private SDK error'));
  await expect(h.task(createTaskContext(), input)).resolves.toMatchObject({
    id: 'workspace-1',
    labelApplied: false
  });
  expect(h.create).toHaveBeenCalledOnce();
  expect(h.close).toHaveBeenCalledOnce();
});

test('connection failures reveal no SDK error and never attempt creation', async () => {
  const h = harness();
  h.connect.mockRejectedValue(new Error('private token in URL'));
  await expect(h.task(createTaskContext(), input)).rejects.toThrow('Could not connect to Paseo.');
  expect(h.create).not.toHaveBeenCalled();
  expect(h.labelWorkspace).not.toHaveBeenCalled();
  expect(h.close).toHaveBeenCalledOnce();
});

test('uncertain writes are not retried and their raw errors are withheld', async () => {
  const h = harness();
  h.create.mockRejectedValue(new Error('private token'));
  await expect(h.task(createTaskContext(), input)).rejects.toThrow(
    'It may have succeeded. Check Paseo'
  );
  expect(h.create).toHaveBeenCalledOnce();
  expect(h.close).toHaveBeenCalledOnce();
});

test.each(['connect', 'create'] as const)(
  'cancellation during %s closes a pending connection',
  async (phase) => {
    const h = harness();
    const controller = new AbortController();
    if (phase === 'connect') h.connect.mockImplementation(() => new Promise(() => {}));
    else h.create.mockImplementation(() => new Promise(() => {}));
    const pending = h.task({ ...createTaskContext(), signal: controller.signal }, input);
    const rejected = expect(pending).rejects.toThrow();
    await vi.waitFor(() => expect(h[phase]).toHaveBeenCalledOnce());
    controller.abort();
    await rejected;
    await vi.waitFor(() => expect(h.close).toHaveBeenCalledOnce());
    if (phase === 'connect') expect(h.create).not.toHaveBeenCalled();
  }
);

test('the deadline releases a pending write without claiming that it failed remotely', async () => {
  const h = harness();
  const deadline = new AbortController();
  const timeout = vi.spyOn(AbortSignal, 'timeout').mockReturnValue(deadline.signal);
  h.create.mockImplementation(() => new Promise(() => {}));
  try {
    const pending = h.task(createTaskContext(), input);
    const rejected = expect(pending).rejects.toThrow('It may have succeeded');
    await vi.waitFor(() => expect(h.create).toHaveBeenCalledOnce());
    deadline.abort();
    await rejected;
    expect(h.close).toHaveBeenCalledOnce();
  } finally {
    timeout.mockRestore();
  }
});

test('an already cancelled task cannot connect', async () => {
  const h = harness();
  await expect(
    h.task({ ...createTaskContext(), signal: AbortSignal.abort() }, input)
  ).rejects.toThrow();
  expect(h.factory).not.toHaveBeenCalled();
});

test('cleanup failure does not lose a confirmed workspace', async () => {
  const h = harness();
  h.close.mockRejectedValue(new Error('Transport already closed'));
  await expect(h.task(createTaskContext(), input)).resolves.toHaveProperty('id', 'workspace-1');
});

test.each(['../escape', '-option', 'branch.lock', 'name with spaces'])(
  'invalid branch %s is rejected before connecting',
  async (branchName) => {
    const h = harness();
    await expect(h.task(createTaskContext(), { ...input, branchName })).rejects.toThrow();
    expect(h.factory).not.toHaveBeenCalled();
  }
);
