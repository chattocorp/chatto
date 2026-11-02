import '../../../../app.css';
import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { MessagesStore } from '@chatto/client/room/messages/MessagesStore';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { mockService } from '@chatto/client/testing/fakeServer';
import { messageEvent } from './messageEventFixture';
import MessageEventTestHarness from './MessageEventTestHarness.svelte';

vi.mock('$lib/client', async () => {
  const { clientMockDefaults } = await import('$lib/test-utils/clientMock');
  return {
    ...clientMockDefaults,
    serverRegistry: {
      ...clientMockDefaults.serverRegistry,
      isOriginServer: () => false,
      getServer: () => ({ id: 'remote-server', url: 'https://example.test' })
    }
  };
});
vi.mock('$app/navigation', () => ({ goto: vi.fn(), pushState: vi.fn() }));
vi.mock('$lib/hooks', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/hooks')>()),
  useMessageActions: () => ({
    addReaction: vi.fn(),
    removeReaction: vi.fn(),
    toggleReaction: vi.fn(),
    startEdit: vi.fn(),
    openDeleteConfirmation: vi.fn(),
    copyMessageText: vi.fn(),
    copyMessageLink: vi.fn()
  })
}));
afterEach(() => vi.restoreAllMocks());

it.each(['svg', 'mermaid'])(
  'keeps %s mounted through payload replacement and reuses it after remount',
  async (language) => {
    const rooms = mockService(RoomService);
    rooms.getRoomEvents.mockResolvedValue({});
    const server = createTestServerScope({
      routes: (router) => router.service(RoomService, rooms)
    });
    const messageStore = new MessagesStore(server.scope.connection, () => null, {
      roomId: 'room-1'
    });
    await expect.poll(() => messageStore.isInitialLoading).toBe(false);
    const body =
      '```' +
      language +
      ' rendered\n' +
      (language === 'svg'
        ? '<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"/>'
        : 'flowchart LR\nA[Reaction]-->B[Stable diagram]') +
      '\n```';
    const event = messageEvent({ body });
    messageStore.events = [event];
    const jobs = vi.spyOn(Worker.prototype, 'postMessage');
    const view = render(MessageEventTestHarness, { event, messageStore });
    const image = () => view.container.querySelector<HTMLImageElement>('[data-canvas-image]');
    try {
      await expect.poll(() => image()?.naturalWidth).toBeGreaterThan(0);
      const original = image()!;
      const frame = original.closest('[data-artifact]');
      jobs.mockClear();
      messageStore.beginOptimisticReaction({
        messageEventId: event.id,
        emoji: 'thumbsup',
        action: 'remove'
      });
      expect(messageStore.events[0].event).not.toBe(event.event);
      await view.rerender({ event: messageStore.events[0] });
      expect(image()).toBe(original);
      expect(original.closest('[data-artifact]')).toBe(frame);
      messageStore.setThreadRootFollowState(event.id, true);
      await view.rerender({ event: messageStore.events[0] });
      expect(image()).toBe(original);
      expect(jobs).not.toHaveBeenCalled();
      await view.rerender({ showMessage: false });
      await view.rerender({ showMessage: true });
      await expect.poll(image).toBe(original);
      expect(jobs).not.toHaveBeenCalled();
    } finally {
      await view.unmount();
      messageStore.dispose();
    }
  }
);
