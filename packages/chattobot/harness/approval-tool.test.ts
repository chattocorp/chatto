import { expect, test, vi } from 'vitest';
import { Type } from 'runling';
import { requireApproval, type ApprovableTool } from './approval-tool.ts';
import { APPROVE, Approvals, DECLINE } from './approvals.ts';
import { Jobs, type JobOutcome } from './jobs.ts';

const thread = { roomId: 'room', threadRootId: 'root' };

function setup() {
  const ran: unknown[] = [];
  const posts: string[] = [];
  const outcomes: JobOutcome[] = [];
  const jobs = new Jobs({
    progress: () => {},
    finished: (_job, outcome) => void outcomes.push(outcome)
  });
  const approvals = new Approvals({
    approvers: new Set(['maintainer']),
    post: async (_destination, text) => {
      posts.push(text);
      return `request-${posts.length}`;
    }
  });
  const tool: ApprovableTool = {
    name: 'createIssue',
    label: 'File issue',
    description: 'File an issue.',
    parameters: Type.Object({ title: Type.String() }),
    describe: ({ title }: { title: string }) => `File issue: ${title}`,
    async execute(_id, input) {
      ran.push(input);
      return {
        content: [{ type: 'text', text: 'Issue filed: https://example.com/1' }],
        details: {}
      };
    }
  };
  const wrapped = requireApproval(tool, {
    jobs,
    approvals,
    replyTo: () => thread,
    signal: new AbortController().signal
  });
  const call = (input: object) =>
    wrapped.execute('call', input as never, undefined, undefined, {} as never);
  return { call, ran, posts, outcomes, approvals, wrapped };
}

test('runs the tool only after an approver approved the exact action', async () => {
  const { call, ran, posts, outcomes, approvals, wrapped } = setup();
  expect(wrapped.description).toContain('A maintainer must approve each call');
  const result = await call({ title: 'Crash' });
  expect(result.content[0]).toMatchObject({ text: expect.stringContaining('Approval requested') });
  await vi.waitFor(() => expect(posts[0]).toContain('File issue: Crash'));
  expect(ran).toEqual([]);
  approvals.react({ messageId: 'request-1', emoji: APPROVE, authorId: 'maintainer' });
  await vi.waitFor(() => expect(outcomes).toHaveLength(1));
  expect(ran).toEqual([{ title: 'Crash' }]);
  expect(outcomes[0]).toEqual({
    result: 'File issue: approved and done. Issue filed: https://example.com/1'
  });
});

test('does not run a declined call', async () => {
  const { call, ran, posts, outcomes, approvals } = setup();
  await call({ title: 'Spam' });
  await vi.waitFor(() => expect(posts).toHaveLength(1));
  approvals.react({ messageId: 'request-1', emoji: DECLINE, authorId: 'maintainer' });
  await vi.waitFor(() => expect(outcomes).toHaveLength(1));
  expect(ran).toEqual([]);
  expect(outcomes[0]).toEqual({ result: 'File issue: not run, because it was declined.' });
});
