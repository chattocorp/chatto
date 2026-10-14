import { expect, test, vi } from 'vitest';
import { APPROVE, Approvals, DECLINE } from './approvals.ts';

const thread = { roomId: 'room', threadRootId: 'root' };

function approvals(approvers = ['maintainer'], timeoutMs?: number) {
  const posts: string[] = [];
  const instance = new Approvals({
    approvers: new Set(approvers),
    post: async (_destination, text) => {
      posts.push(text);
      return `request-${posts.length}`;
    },
    ...(timeoutMs ? { timeoutMs } : {})
  });
  return { instance, posts };
}

test('posts the exact action and waits for an approver', async () => {
  const { instance, posts } = approvals();
  const decision = instance.request('File issue X', thread, new AbortController().signal);
  await vi.waitFor(() => expect(posts).toHaveLength(1));
  expect(posts[0]).toBe(
    'File issue X\n\nA maintainer can approve this with ✅ or decline it with ❌.'
  );
  // Reactions from others, other emoji, and other messages do not decide.
  expect(instance.react({ messageId: 'request-1', emoji: APPROVE, authorId: 'alice' })).toBe(false);
  expect(instance.react({ messageId: 'request-1', emoji: 'eyes', authorId: 'maintainer' })).toBe(
    false
  );
  expect(instance.react({ messageId: 'other', emoji: APPROVE, authorId: 'maintainer' })).toBe(
    false
  );
  expect(instance.react({ messageId: 'request-1', emoji: APPROVE, authorId: 'maintainer' })).toBe(
    true
  );
  expect(await decision).toBe('approved');
  // A decided request takes no more reactions.
  expect(instance.react({ messageId: 'request-1', emoji: DECLINE, authorId: 'maintainer' })).toBe(
    false
  );
});

test('a decline, a timeout, or missing approvers do not approve', async () => {
  const { instance, posts } = approvals(['maintainer'], 50);
  const declined = instance.request('A', thread, new AbortController().signal);
  await vi.waitFor(() => expect(posts).toHaveLength(1));
  instance.react({ messageId: 'request-1', emoji: DECLINE, authorId: 'maintainer' });
  expect(await declined).toBe('declined');
  expect(await instance.request('B', thread, new AbortController().signal)).toBe('expired');
  expect(await approvals([]).instance.request('C', thread, new AbortController().signal)).toBe(
    'declined'
  );
});

test('cancellation rejects a waiting request', async () => {
  const { instance } = approvals();
  const controller = new AbortController();
  const decision = instance.request('A', thread, controller.signal);
  controller.abort(new Error('Stopped'));
  await expect(decision).rejects.toThrow('Stopped');
});
