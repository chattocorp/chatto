export type BottomScrollToken = {
  operationId: number;
  roomId: string;
  intentRevision: number;
};

export type TimelineScrollObservation = {
  offset: number;
  scrollSize: number;
  viewportSize: number;
  firstVisibleAt: string | null;
  now: number;
};

export type TimelineScrollResult = {
  distanceFromBottom: number;
  reachedBottom: boolean;
};

const USER_SCROLL_INTENT_MS = 250;
const SCROLL_UP_LOCK_MS = 150;

/**
 * Owns timeline viewport intent independently of DOM and Virtua operations.
 *
 * Components report explicit input transitions and use bottom-scroll tokens
 * to fence async DOM work. Browser measurements, timers, and scrolling remain
 * at the component boundary.
 */
export class TimelineViewportController {
  initialScrollDone = $state(false);
  shouldScrollToBottom = $state(true);
  hasNewMessages = $state(false);
  firstVisibleAt = $state<string | null>(null);

  #timelineKey: string | null = null;
  #lastSeenNewestId: string | null = null;
  #previousOffset: number | null = null;
  #userScrollIntentAt = 0;
  #intentRevision = 0;
  #scrollUpLockedUntil = 0;
  #bottomScrollOperation = 0;
  #wasJumpedMode = false;
  /**
   * One-shot landing on the unread separator for `#unreadLandingKey`.
   * `armed` from room entry until the landing starts; `running` until it
   * settles. Any explicit viewport action (user scroll, jump, post, jump to
   * present) resets it to `idle`. The state is keyed by timeline, so a
   * decision made during mount stays valid when `enterRoom` for that timeline
   * runs later.
   */
  #unreadLanding: 'idle' | 'armed' | 'running' = 'idle';
  #unreadLandingKey: string | null = null;

  /**
   * Reset viewport intent when the timeline shows a different conversation.
   * The key is the room id, plus the thread root id for a thread timeline.
   * Returns false when the key is unchanged.
   */
  enterRoom(timelineKey: string): boolean {
    if (timelineKey === this.#timelineKey) return false;

    this.#timelineKey = timelineKey;
    this.cancelBottomScroll();
    this.initialScrollDone = false;
    this.followBottom();
    this.#lastSeenNewestId = null;
    this.#previousOffset = null;
    this.#scrollUpLockedUntil = 0;
    this.#wasJumpedMode = false;
    this.#enterUnreadLanding(timelineKey);
    return true;
  }

  /** Arm the landing when the given timeline has no landing decision yet. */
  #enterUnreadLanding(timelineKey: string): void {
    if (this.#unreadLandingKey === timelineKey) return;
    this.#unreadLandingKey = timelineKey;
    this.#unreadLanding = 'armed';
  }

  followBottom(): void {
    this.shouldScrollToBottom = true;
    this.hasNewMessages = false;
    this.firstVisibleAt = null;
  }

  stopFollowingBottom(): void {
    this.shouldScrollToBottom = false;
  }

  observeJumpedMode(isJumpedMode: boolean): void {
    if (this.#wasJumpedMode && !isJumpedMode) this.followBottom();
    this.#wasJumpedMode = isJumpedMode;
  }

  observeNewestEvent(newestId: string | null): void {
    if (newestId === null) return;
    if (
      this.#lastSeenNewestId !== null &&
      newestId !== this.#lastSeenNewestId &&
      !this.shouldScrollToBottom
    ) {
      this.hasNewMessages = true;
    }
    this.#lastSeenNewestId = newestId;
  }

  /**
   * Explicit user request to see the latest messages, such as posting or
   * clicking the jump button. It supersedes an unread separator landing and
   * the short virtualizer-correction lock.
   */
  requestBottom(): void {
    this.#unreadLanding = 'idle';
    this.followBottom();
    this.unlockScrollUp();
  }

  beginJump(): void {
    this.#unreadLanding = 'idle';
    this.cancelBottomScroll();
    this.stopFollowingBottom();
    this.initialScrollDone = true;
  }

  settleJump(distanceFromBottom: number): void {
    if (distanceFromBottom < 50) this.followBottom();
  }

  prepareJumpToPresent(): void {
    this.#unreadLanding = 'idle';
    this.cancelBottomScroll();
    this.followBottom();
    this.initialScrollDone = false;
    this.unlockScrollUp();
  }

  markUserScrollIntent(now = Date.now()): void {
    this.#userScrollIntentAt = now;
    this.#intentRevision += 1;
    this.#unreadLanding = 'idle';
    this.cancelBottomScroll();
  }

  /**
   * Start the one-shot landing on the unread separator for this room entry.
   *
   * Returns true at most once per timeline entry, and only when no explicit
   * viewport action happened since entry. A marker that appears later, for
   * example after the app returns to the foreground, does not move the view.
   * While the landing runs, scroll observations cannot re-enable bottom
   * following: late events from the superseded bottom scroll would otherwise
   * report the old bottom position.
   */
  beginUnreadEntryLanding(timelineKey: string): boolean {
    this.#enterUnreadLanding(timelineKey);
    if (this.#unreadLanding !== 'armed') return false;
    this.beginJump();
    this.#unreadLanding = 'running';
    return true;
  }

  /**
   * Skip the landing for this entry. Use it when the entry targets a specific
   * message, so that message stays in view even if its jump never starts.
   */
  skipUnreadEntryLanding(timelineKey: string): void {
    this.#enterUnreadLanding(timelineKey);
    this.#unreadLanding = 'idle';
  }

  /** False once another viewport action superseded the running landing. */
  get isUnreadEntryLandingRunning(): boolean {
    return this.#unreadLanding === 'running';
  }

  /**
   * Finish a landing from the measured final position. Near the bottom, the
   * timeline follows new messages again. Otherwise, the scroll-up lock keeps
   * trailing virtualizer corrections from undoing the landing.
   */
  finishUnreadEntryLanding(distanceFromBottom: number | null, now = Date.now()): void {
    if (this.#unreadLanding !== 'running') return;
    this.#unreadLanding = 'idle';
    if (distanceFromBottom === null) return;
    if (distanceFromBottom < 50) {
      this.followBottom();
    } else {
      this.stopFollowingBottom();
      this.#scrollUpLockedUntil = now + SCROLL_UP_LOCK_MS;
    }
  }

  captureIntentRevision(): number {
    return this.#intentRevision;
  }

  hasIntentRevision(revision: number): boolean {
    return revision === this.#intentRevision;
  }

  observeScroll(observation: TimelineScrollObservation): TimelineScrollResult {
    const distanceFromBottom =
      observation.scrollSize - observation.offset - observation.viewportSize;
    let reachedBottom = false;

    const scrollUpLocked =
      this.#unreadLanding === 'running' || observation.now < this.#scrollUpLockedUntil;
    if (distanceFromBottom < 10 && !scrollUpLocked) {
      const wasScrolledUp = !this.shouldScrollToBottom;
      this.followBottom();
      reachedBottom =
        wasScrolledUp && observation.now - this.#userScrollIntentAt < USER_SCROLL_INTENT_MS;
    } else if (
      observation.now - this.#userScrollIntentAt < USER_SCROLL_INTENT_MS &&
      this.#previousOffset !== null &&
      observation.offset < this.#previousOffset - 10 &&
      distanceFromBottom > 20
    ) {
      this.stopFollowingBottom();
      this.cancelBottomScroll();
      this.#scrollUpLockedUntil = observation.now + SCROLL_UP_LOCK_MS;
    }

    this.#previousOffset = observation.offset;
    if (!this.shouldScrollToBottom && observation.firstVisibleAt) {
      this.firstVisibleAt = observation.firstVisibleAt;
    }

    return { distanceFromBottom, reachedBottom };
  }

  reconcileAfterTabResume(distanceFromBottom: number): void {
    if (!this.shouldScrollToBottom || !this.initialScrollDone) return;
    if (distanceFromBottom > 50) this.stopFollowingBottom();
  }

  beginBottomScroll(roomId: string): BottomScrollToken {
    return {
      operationId: ++this.#bottomScrollOperation,
      roomId,
      intentRevision: this.#intentRevision
    };
  }

  canContinueBottomScroll(
    token: BottomScrollToken,
    currentRoomId: string,
    isJumpedMode: boolean
  ): boolean {
    return (
      token.operationId === this.#bottomScrollOperation &&
      token.roomId === currentRoomId &&
      token.intentRevision === this.#intentRevision &&
      !isJumpedMode &&
      this.shouldScrollToBottom
    );
  }

  completeBottomScroll(token: BottomScrollToken): void {
    if (token.operationId === this.#bottomScrollOperation) {
      this.initialScrollDone = true;
    }
  }

  cancelBottomScroll(): void {
    this.#bottomScrollOperation += 1;
  }

  private unlockScrollUp(): void {
    this.#scrollUpLockedUntil = 0;
  }
}
