import { describe, expect, it } from 'vitest';
import { TimelineViewportController } from './TimelineViewportController.svelte';

describe('TimelineViewportController', () => {
  it('resets viewport intent exactly once when entering a room', () => {
    const controller = new TimelineViewportController();
    controller.enterRoom('R1');
    controller.stopFollowingBottom();
    controller.hasNewMessages = true;
    controller.firstVisibleAt = '2026-01-01T00:00:00Z';
    controller.initialScrollDone = true;

    expect(controller.enterRoom('R1')).toBe(false);
    expect(controller.shouldScrollToBottom).toBe(false);

    expect(controller.enterRoom('R2')).toBe(true);
    expect(controller.shouldScrollToBottom).toBe(true);
    expect(controller.hasNewMessages).toBe(false);
    expect(controller.firstVisibleAt).toBeNull();
    expect(controller.initialScrollDone).toBe(false);
  });

  it('shows new-message state only when a newer tail arrives while scrolled up', () => {
    const controller = new TimelineViewportController();
    controller.enterRoom('R1');
    controller.observeNewestEvent('M1', {
      showNewMessagesIndicator: true,
      alwaysScrollToBottom: false
    });
    controller.stopFollowingBottom();

    controller.observeNewestEvent('M2', {
      showNewMessagesIndicator: true,
      alwaysScrollToBottom: false
    });

    expect(controller.hasNewMessages).toBe(true);
    controller.followBottom();
    expect(controller.hasNewMessages).toBe(false);
  });

  it('distinguishes user scroll-up from virtualizer corrections', () => {
    const controller = new TimelineViewportController();
    controller.enterRoom('R1');

    controller.observeScroll({
      offset: 700,
      scrollSize: 1_000,
      viewportSize: 300,
      firstVisibleAt: null,
      alwaysScrollToBottom: false,
      now: 1_000
    });
    controller.observeScroll({
      offset: 500,
      scrollSize: 1_000,
      viewportSize: 300,
      firstVisibleAt: '2026-01-01T00:00:00Z',
      alwaysScrollToBottom: false,
      now: 1_100
    });
    expect(controller.shouldScrollToBottom).toBe(true);

    controller.markUserScrollIntent(2_000);
    controller.observeScroll({
      offset: 400,
      scrollSize: 1_000,
      viewportSize: 300,
      firstVisibleAt: '2026-01-02T00:00:00Z',
      alwaysScrollToBottom: false,
      now: 2_010
    });

    expect(controller.shouldScrollToBottom).toBe(false);
    expect(controller.firstVisibleAt).toBe('2026-01-02T00:00:00Z');
  });

  it('reports a user-driven return to bottom after the scroll-up lock expires', () => {
    const controller = new TimelineViewportController();
    controller.enterRoom('R1');
    controller.observeScroll({
      offset: 700,
      scrollSize: 1_000,
      viewportSize: 300,
      firstVisibleAt: null,
      alwaysScrollToBottom: false,
      now: 1_000
    });
    controller.markUserScrollIntent(2_000);
    controller.observeScroll({
      offset: 400,
      scrollSize: 1_000,
      viewportSize: 300,
      firstVisibleAt: null,
      alwaysScrollToBottom: false,
      now: 2_010
    });

    const locked = controller.observeScroll({
      offset: 700,
      scrollSize: 1_000,
      viewportSize: 300,
      firstVisibleAt: null,
      alwaysScrollToBottom: false,
      now: 2_100
    });
    expect(locked.reachedBottom).toBe(false);
    expect(controller.shouldScrollToBottom).toBe(false);

    controller.markUserScrollIntent(2_200);
    const settled = controller.observeScroll({
      offset: 700,
      scrollSize: 1_000,
      viewportSize: 300,
      firstVisibleAt: null,
      alwaysScrollToBottom: false,
      now: 2_210
    });
    expect(settled.reachedBottom).toBe(true);
    expect(controller.shouldScrollToBottom).toBe(true);
  });

  it('fences bottom convergence by operation, room, and user intent', () => {
    const controller = new TimelineViewportController();
    controller.enterRoom('R1');
    const first = controller.beginBottomScroll('R1');
    const second = controller.beginBottomScroll('R1');

    expect(controller.canContinueBottomScroll(first, 'R1', false, false)).toBe(false);
    expect(controller.canContinueBottomScroll(second, 'R1', false, false)).toBe(true);
    expect(controller.canContinueBottomScroll(second, 'R2', false, false)).toBe(false);

    controller.markUserScrollIntent(1_000);
    expect(controller.canContinueBottomScroll(second, 'R1', false, false)).toBe(false);
    controller.completeBottomScroll(second);
    expect(controller.initialScrollDone).toBe(false);
  });

  it('makes jump and composer transitions explicit', () => {
    const controller = new TimelineViewportController();
    controller.enterRoom('R1');

    controller.beginJump();
    expect(controller.shouldScrollToBottom).toBe(false);
    expect(controller.initialScrollDone).toBe(true);

    controller.settleJump(20);
    expect(controller.shouldScrollToBottom).toBe(true);

    controller.stopFollowingBottom();
    controller.requestComposerBottom();
    expect(controller.shouldScrollToBottom).toBe(true);

    controller.prepareJumpToPresent();
    expect(controller.initialScrollDone).toBe(false);
  });

  it('follows the latest window when jumped mode ends', () => {
    const controller = new TimelineViewportController();
    controller.enterRoom('R1');
    controller.observeJumpedMode(true);
    controller.stopFollowingBottom();

    controller.observeJumpedMode(false);

    expect(controller.shouldScrollToBottom).toBe(true);
  });

  it('detects bottom drift after a hidden tab resumes', () => {
    const controller = new TimelineViewportController();
    controller.enterRoom('R1');
    const token = controller.beginBottomScroll('R1');
    controller.completeBottomScroll(token);

    controller.reconcileAfterTabResume(100, false);

    expect(controller.shouldScrollToBottom).toBe(false);
  });
});
