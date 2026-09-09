import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
import MessageEventTestHarness from './MessageEventTestHarness.svelte';

function pendingEvent(): TimelineEventView {
  return {
    id: 'incoming',
    actorId: 'author',
    actor: null,
    actorResolution: 'loading',
    createdAt: '2026-09-09T12:00:00Z',
    event: {
      kind: TimelineEventKind.MessagePosted,
      roomId: 'room-1',
      body: 'Incoming message',
      attachments: [],
      reactions: [],
      replyCount: 0,
      threadParticipants: []
    }
  };
}

describe('realtime message author', () => {
  it.each([false, true])(
    'keeps the body visible while the author loads (bot: %s)',
    async (isBot) => {
      const event = pendingEvent();
      const view = render(MessageEventTestHarness, { event });
      await expect.element(view.getByText('Incoming message')).toBeVisible();
      await expect.element(view.getByText('Loading...')).toBeInTheDocument();
      expect(view.container.querySelector('.skeleton')).not.toBeNull();
      expect(view.container.textContent).not.toContain('[deleted user]');
      expect(view.container.querySelector('[aria-label="[deleted user]"]')).toBeNull();
      await view.rerender({
        event: {
          ...event,
          actorResolution: undefined,
          actor: {
            id: 'author',
            login: 'author',
            displayName: 'Message author',
            deleted: false,
            isBot,
            avatarUrl: null,
            presenceStatus: PresenceStatus.OFFLINE
          }
        }
      });
      await expect.element(view.getByText('Message author', { exact: true })).toBeVisible();
      expect(view.container.querySelector('[aria-busy="true"]')).toBeNull();
      expect(view.container.textContent).not.toContain('[deleted user]');
    }
  );

  it('shows unknown after a failed lookup and deleted after account removal', async () => {
    const event = pendingEvent();
    const view = render(MessageEventTestHarness, { event });
    await view.rerender({ event: { ...event, actorResolution: 'unavailable' } });
    await expect.element(view.getByText('Unknown user', { exact: true })).toBeVisible();
    expect(view.container.querySelector('[aria-busy="true"]')).toBeNull();
    expect(view.container.textContent).not.toContain('[deleted user]');
    await view.rerender({ event: { ...event, actorResolution: 'deleted' } });
    await expect.element(view.getByText('[deleted user]', { exact: true })).toBeVisible();
    await expect.element(view.getByText('Incoming message')).toBeVisible();
  });
});
