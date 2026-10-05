import { expect, test } from 'vitest';
import { createAuthorizationClassifier, type AgentTaskState } from 'runling/agents';
import { createWorkflowContext, emptyTokenUsage } from 'runling';
import { createInvestigationFollowthrough } from './investigation-followthrough.ts';

const completed = (id = 'owned', outcome = 'completed') =>
  ({ id, status: 'completed', result: JSON.stringify({ outcome }) }) as AgentTaskState;

test('retains the original request, checks observed completion, and reserves once across notifications', () => {
  const follow = createInvestigationFollowthrough();
  const create = ['issue', 'create', '--milestone', '0.5.0'];
  follow.started('owned', ['Investigate and file a bug, milestone 0.5.0.']);
  for (const state of [
    completed('other'),
    completed('owned', 'failed'),
    { ...completed(), status: 'running' as const },
    { ...completed(), result: 'invalid' }
  ]) {
    follow.select(state.id, [state]);
    expect(follow.canFile(create)).toBe(false);
    expect(() => follow.reserve(create)).toThrow();
  }
  follow.select('owned', [completed()]);
  follow.update('This is about preview cards.');
  expect(follow.messages([])).toEqual([
    'Investigate and file a bug, milestone 0.5.0.',
    'This is about preview cards.'
  ]);
  expect(follow.canFile(['issue', 'close', '12'])).toBe(false);
  expect(follow.canFile(create)).toBe(true);
  follow.reserve(create);
  // Even an uncertain transport failure must not permit another create.
  follow.select('owned', [completed()]);
  expect(follow.canFile(create)).toBe(false);
  expect(() => follow.reserve(create)).toThrow();
});

test('keeps withdrawal beyond recent-message history and fails closed at the retention limit', () => {
  const follow = createInvestigationFollowthrough();
  follow.started('owned', ['File an issue.']);
  follow.update('Do not file this issue.');
  for (let i = 0; i < 15; i++) follow.update('Other discussion.');
  follow.select('owned', [completed()]);
  expect(follow.messages([])).toContain('Do not file this issue.');
  for (let i = 0; i < 100; i++) follow.update('More discussion.');
  expect(follow.canFile(['issue', 'create'])).toBe(false);
  expect(() => follow.reserve(['issue', 'create'])).toThrow();
});

test('retained messages reach the actual classifier prompt without losing a late withdrawal', async () => {
  const follow = createInvestigationFollowthrough();
  follow.started('owned', ['Investigate and file a bug.']);
  const withdrawal = 'x'.repeat(3_970) + ' Do not file this issue.';
  follow.update(withdrawal);
  for (let i = 0; i < 18; i++) follow.update(`Discussion ${i}`);
  follow.select('owned', [completed()]);
  expect(follow.canFile(['issue', 'create'])).toBe(true);
  const messages = follow.messages([]);
  let classifierMessages: unknown;
  const classify = createAuthorizationClassifier({
    model: 'test/model',
    createAgent: async () => ({
      dispose() {},
      async runOutcome(_ctx, prompt) {
        classifierMessages = JSON.parse(prompt).messagesFromAuthorizedPeople;
        return { outcome: 'completed', summary: '', usage: emptyTokenUsage() };
      }
    })
  });
  await classify(createWorkflowContext(), { action: 'issue create', messages });
  expect(classifierMessages).toEqual(messages);
  expect(classifierMessages).toContain(withdrawal);
  follow.update('One more message.');
  expect(follow.canFile(['issue', 'create'])).toBe(false);
});

test.each(['initial', 'later'])(
  'a long %s message disables filing instead of truncating its withdrawal',
  (when) => {
    const follow = createInvestigationFollowthrough();
    const long = 'File a bug. ' + 'x'.repeat(4_000) + ' Do not file it.';
    follow.started('owned', [when === 'initial' ? long : 'File a bug.']);
    if (when === 'later') follow.update(long);
    follow.select('owned', [completed()]);
    expect(follow.canFile(['issue', 'create'])).toBe(false);
    expect(() => follow.reserve(['issue', 'create'])).toThrow();
  }
);
