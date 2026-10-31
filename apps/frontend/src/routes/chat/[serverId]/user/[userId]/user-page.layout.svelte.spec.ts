import { flushSync } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
  params: { threadId: undefined, messageId: undefined } as Record<string, string | undefined>,
  invalidate: vi.fn()
}));
vi.mock('$app/state', () => ({
  page: {
    route: { id: '/chat/[serverId]/user/[userId]' },
    get params() {
      return mocks.params;
    }
  }
}));
vi.mock('$app/navigation', () => ({
  invalidate: mocks.invalidate,
  goto: vi.fn(),
  pushState: vi.fn()
}));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
vi.mock('../../[roomId]/Room.svelte', async () => ({
  default: (await import('$lib/test-utils/ConversationStub.svelte')).default
}));
import Destination from './+layout.svelte';
import type { LayoutData } from './$types';

function data(roomId: string | null): LayoutData {
  return { conversationRoomId: Promise.resolve(roomId) } as LayoutData;
}

beforeEach(() => vi.resetAllMocks());

describe('user page', () => {
  it('ignores a late result after another recipient replaces the pending conversation', async () => {
    let finish!: (roomId: string) => void;
    const pending = new Promise<string>((resolve) => {
      finish = resolve;
    });
    const view = render(Destination, { data: { conversationRoomId: pending } as LayoutData });
    await expect.element(view.getByRole('status', { name: 'Loading...' })).toBeInTheDocument();
    await view.rerender({ data: data('new-room') });
    await expect.element(view.getByText('Conversation: new-room')).toBeVisible();
    finish('old-room');
    await pending;
    flushSync();
    await expect.element(view.getByText('Conversation: old-room')).not.toBeInTheDocument();
  });
  it('keeps the conversation mounted across thread and message navigation', async () => {
    const resolved = data('dm-room');
    const view = render(Destination, { data: resolved });
    const conversation = view.getByText('Conversation: dm-room');
    await expect.element(conversation).toBeVisible();
    const original = conversation.element();
    for (const threadId of ['thread-1', 'thread-2', undefined]) {
      mocks.params = { threadId, messageId: 'message' };
      await view.rerender({ data: resolved });
      flushSync();
      expect(conversation.element()).toBe(original);
    }
  });

  it('retries through route invalidation and renders the new result', async () => {
    let finish!: () => void;
    mocks.invalidate.mockReturnValue(
      new Promise<void>((resolve) => {
        finish = resolve;
      })
    );
    const view = render(Destination, { data: data(null) });
    const retry = view.getByRole('button', { name: 'Try Again' });
    await retry.click();
    expect(mocks.invalidate).toHaveBeenCalledExactlyOnceWith('chatto:conversation');
    await view.rerender({ data: data('dm-room') });
    finish();
    await expect.element(view.getByText('Conversation: dm-room')).toBeVisible();
  });
});
