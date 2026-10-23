import { expect, test } from 'vitest';
import { buildTimeline, isActivityActive, activityStatus, type Activity } from './timeline.ts';
import type { RunlingEvent } from 'runling';

const task = (overrides: Partial<Activity> = {}): Activity => ({
  id: 'task',
  label: 'Task',
  kind: 'step',
  status: 'running',
  startedAt: 0,
  logs: [],
  children: [],
  ...overrides
});

test('indicates running leaf work but not waits or finished tasks', () => {
  for (const kind of ['step', 'agent', 'command'] as const)
    expect(isActivityActive(task({ kind }))).toBe(true);
  expect(isActivityActive(task({ kind: 'wait' }))).toBe(false);
  for (const status of ['completed', 'failed', 'blocked', 'interrupted'])
    expect(isActivityActive(task({ status }))).toBe(false);
});

test('keeps waiting ancestors quiet and marks parallel children active', () => {
  const first = task({ id: 'first', kind: 'agent' });
  const second = task({ id: 'second', kind: 'command' });
  const parent = task({ children: [first, second] });
  expect(isActivityActive(parent)).toBe(false);
  expect(isActivityActive(task({ children: [parent] }))).toBe(false);
  expect(isActivityActive(first)).toBe(true);
  expect(isActivityActive(second)).toBe(true);
  first.status = 'completed';
  expect(isActivityActive(parent)).toBe(false);
  second.status = 'completed';
  expect(isActivityActive(parent)).toBe(true);
});

test('does not animate a task that waits for a message', () => {
  expect(isActivityActive(task({ children: [task({ kind: 'wait' })] }))).toBe(false);
});

test('uses formatted logs once without deduplicating genuine repeated messages', () => {
  const events: RunlingEvent[] = [
    {
      type: 'agent.started',
      agentId: 'a',
      model: 'model',
      color: 'blue',
      timestamp: 0
    }
  ];
  const message = '\u001b[33m[a]\u001b[0m Reading file.ts';
  for (let i = 0; i < 2; i++) {
    events.push(
      {
        type: 'agent.action',
        agentId: 'a',
        action: 'Reading file.ts',
        timestamp: i * 2 + 1
      },
      {
        type: 'log',
        source: 'agent',
        sourceId: 'a',
        message,
        level: 'info',
        depth: 0,
        color: 'blue',
        timestamp: i * 2 + 2
      }
    );
  }
  const node = buildTimeline(events, 'running')[0]!;
  expect(node.logs).toEqual([message, message]);
  expect(node.preview).toBe('Reading file.ts');
  // A later invocation of the same agent can still have action-only history.
  events.push(
    {
      type: 'agent.started',
      agentId: 'a',
      model: 'model',
      color: 'blue',
      timestamp: 5
    },
    {
      type: 'agent.action',
      agentId: 'a',
      action: 'Legacy message',
      timestamp: 6
    }
  );
  expect(buildTimeline(events, 'completed')[1]?.logs).toEqual(['Legacy message']);
});

test('shows legacy tool actions without letting debug or token summaries replace them', () => {
  const events: RunlingEvent[] = [
    {
      type: 'agent.started',
      agentId: 'a',
      model: 'model',
      color: 'blue',
      timestamp: 0
    },
    ...[
      'Agent started (model: model)',
      'Reading\n src/file.ts',
      'Turn 2 started',
      'Token usage: in 10, out 2'
    ].map((action, index) => ({
      type: 'agent.action' as const,
      agentId: 'a',
      action,
      timestamp: index + 1
    }))
  ];
  expect(buildTimeline(events, 'completed')[0]?.preview).toBe('Reading src/file.ts');
  expect(buildTimeline(events, 'running')[0]?.logs).toHaveLength(4);
});

test('keeps agent previews separate from logs and retains them on completion', () => {
  const nodes = buildTimeline(
    [
      {
        type: 'agent.started',
        agentId: 'a',
        model: 'model',
        color: 'blue',
        timestamp: 0
      },
      {
        type: 'agent.started',
        agentId: 'b',
        model: 'model',
        color: 'blue',
        timestamp: 1
      },
      {
        type: 'agent.progress',
        agentId: 'a',
        text: 'Reading file.ts',
        timestamp: 2
      },
      {
        type: 'agent.progress',
        agentId: 'b',
        text: 'Running tests',
        timestamp: 3
      },
      {
        type: 'agent.action',
        agentId: 'a',
        action: 'Turn 2 started',
        timestamp: 4
      },
      {
        type: 'agent.finished',
        agentId: 'a',
        outcome: 'completed',
        usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
        timestamp: 5
      }
    ],
    'running'
  );
  expect(nodes[0]?.preview).toBe('Reading file.ts');
  expect(nodes[0]?.logs).toEqual(['Turn 2 started']);
  expect(nodes[1]?.preview).toBe('Running tests');
});

test('rolls live snapshots through nested steps without double counting finishes or parallel agents', () => {
  const usage = {
    input: 10,
    output: 2,
    cacheRead: 30,
    cacheWrite: 1,
    cost: 0.01
  };
  const events: RunlingEvent[] = [
    { type: 'step.started', id: 'root', label: 'Root', timestamp: 0 },
    {
      type: 'step.started',
      id: 'child',
      label: 'Child',
      activityId: 'root',
      timestamp: 1
    },
    {
      type: 'agent.started',
      agentId: 'a',
      model: 'model',
      color: 'blue',
      activityId: 'child',
      timestamp: 2
    },
    {
      type: 'agent.started',
      agentId: 'b',
      model: 'model',
      color: 'blue',
      activityId: 'root',
      timestamp: 3
    },
    { type: 'agent.usage', agentId: 'a', usage, timestamp: 4 },
    {
      type: 'agent.usage',
      agentId: 'a',
      usage: { ...usage, input: 20 },
      timestamp: 5
    },
    { type: 'agent.usage', agentId: 'b', usage, timestamp: 6 }
  ];
  const live = buildTimeline(events, 'running')[0]!;
  expect(live.usage).toEqual({
    ...usage,
    input: 30,
    output: 4,
    cacheRead: 60,
    cacheWrite: 2,
    cost: 0.02
  });
  expect(live.children[0]?.usage?.input).toBe(20);
  events.push({
    type: 'agent.finished',
    agentId: 'a',
    outcome: 'failed',
    usage: { ...usage, input: 20 },
    timestamp: 7
  });
  expect(buildTimeline(events, 'interrupted')[0]?.usage).toEqual(live.usage);
  // Older journals have only agent.finished events.
  expect(
    buildTimeline(
      events.filter((e) => e.type !== 'agent.usage'),
      'failed'
    )[0]?.usage?.input
  ).toBe(20);
});

test('keeps repeated turns of one agent distinct and interrupts unfinished blocks', () => {
  const tree = buildTimeline(
    [
      { type: 'step.started', id: 'parent', label: 'Parent', timestamp: 0 },
      {
        type: 'agent.started',
        agentId: 'agent',
        model: 'model',
        color: 'blue',
        activityId: 'parent',
        timestamp: 1
      },
      {
        type: 'agent.finished',
        agentId: 'agent',
        outcome: 'completed',
        timestamp: 5,
        usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }
      },
      {
        type: 'agent.started',
        agentId: 'agent',
        model: 'model',
        color: 'blue',
        activityId: 'parent',
        timestamp: 6
      }
    ],
    'interrupted'
  );
  expect(tree[0]?.status).toBe('interrupted');
  expect(tree[0]?.children.map((node) => node.status)).toEqual(['completed', 'interrupted']);
});

test('a wait is a child activity of the task that waits', () => {
  const events: RunlingEvent[] = [
    { type: 'step.started', id: 'task', label: 'Chat', timestamp: 0 },
    { type: 'wait.started', id: 'w', channelId: 'c', activityId: 'task', timestamp: 10 }
  ];
  const [running] = buildTimeline(events, 'running');
  expect(running!.children[0]).toMatchObject({ kind: 'wait', status: 'running', startedAt: 10 });
  expect(activityStatus(running!.children[0]!)).toBe('waiting for a message');
  expect(isActivityActive(running!)).toBe(false);

  for (const [status, label] of [
    ['received', 'message received'],
    ['closed', 'closed']
  ] as const) {
    const [task] = buildTimeline(
      [
        ...events,
        { type: 'wait.finished', id: 'w', status, durationMs: 500, timestamp: 510 },
        { type: 'step.finished', id: 'task', status: 'completed', durationMs: 510, timestamp: 510 }
      ],
      'completed'
    );
    expect(task!.children[0]!.durationMs).toBe(500);
    expect(activityStatus(task!.children[0]!)).toBe(label);
  }

  const [stopped] = buildTimeline(events, 'interrupted');
  expect(stopped!.children[0]!.status).toBe('interrupted');
});

test('message markers resolve delayed task links and distinguish queued and read messages', () => {
  const nodes = buildTimeline(
    [
      { type: 'step.started', id: 'parent', label: 'Coordinator', timestamp: 0 },
      {
        type: 'message.sent',
        id: 'q',
        channelId: 'channel',
        direction: 'input',
        payload: 'queued',
        activityId: 'parent',
        timestamp: 1
      },
      {
        type: 'step.started',
        id: 'child',
        label: 'Investigator',
        activityId: 'parent',
        timestamp: 2
      },
      { type: 'task.linked', channelId: 'channel', taskId: 'child', timestamp: 2 },
      {
        type: 'message.sent',
        id: 'r',
        channelId: 'channel',
        direction: 'input',
        payload: 'read',
        activityId: 'parent',
        timestamp: 3
      },
      { type: 'message.read', id: 'r', timestamp: 4 },
      {
        type: 'message.sent',
        id: 'u',
        channelId: 'channel',
        direction: 'update',
        payload: 'result',
        activityId: 'parent',
        timestamp: 7
      },
      { type: 'message.read', id: 'u', timestamp: 8 }
    ],
    'running'
  );
  const messages = nodes[0]!.children[0]!.messages!;
  expect(messages.map((message) => message.status)).toEqual(['Queued', 'Read', 'Read']);
  expect(messages[0]).toMatchObject({ from: 'Coordinator', to: 'Investigator' });
  expect(messages[2]).toMatchObject({ from: 'Investigator', to: 'Coordinator' });
});

test('messages from the host show on the top-level task', () => {
  const [root] = buildTimeline(
    [
      { type: 'step.started', id: 'root', label: 'Chat', timestamp: 0 },
      { type: 'task.linked', channelId: 'host', taskId: 'root', timestamp: 0 },
      {
        type: 'message.sent',
        id: 'm',
        channelId: 'host',
        direction: 'input',
        payload: 'Hello',
        timestamp: 1
      },
      {
        type: 'message.sent',
        id: 'u',
        channelId: 'host',
        direction: 'update',
        payload: 'Hi',
        timestamp: 2
      }
    ],
    'running'
  );
  expect(root!.messages).toEqual([
    expect.objectContaining({ from: 'Host', to: 'Chat', payload: 'Hello', status: 'Queued' }),
    expect.objectContaining({ from: 'Chat', to: 'Host', payload: 'Hi' })
  ]);
});
