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
  alwaysScrollToBottom: boolean;
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

  #roomId: string | null = null;
  #lastSeenNewestId: string | null = null;
  #previousOffset: number | null = null;
  #userScrollIntentAt = 0;
  #intentRevision = 0;
  #scrollUpLockedUntil = 0;
  #bottomScrollOperation = 0;
  #wasJumpedMode = false;

  enterRoom(roomId: string): boolean {
    if (roomId === this.#roomId) return false;

    this.#roomId = roomId;
    this.cancelBottomScroll();
    this.initialScrollDone = false;
    this.followBottom();
    this.#lastSeenNewestId = null;
    this.#previousOffset = null;
    this.#scrollUpLockedUntil = 0;
    this.#wasJumpedMode = false;
    return true;
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

  observeNewestEvent(
    newestId: string | null,
    options: { showNewMessagesIndicator: boolean; alwaysScrollToBottom: boolean }
  ): void {
    if (!options.showNewMessagesIndicator || options.alwaysScrollToBottom || newestId === null) {
      return;
    }
    if (
      this.#lastSeenNewestId !== null &&
      newestId !== this.#lastSeenNewestId &&
      !this.shouldScrollToBottom
    ) {
      this.hasNewMessages = true;
    }
    this.#lastSeenNewestId = newestId;
  }

  requestComposerBottom(): void {
    this.followBottom();
    this.unlockScrollUp();
  }

  beginJump(): void {
    this.cancelBottomScroll();
    this.stopFollowingBottom();
    this.initialScrollDone = true;
  }

  settleJump(distanceFromBottom: number): void {
    if (distanceFromBottom < 50) this.followBottom();
  }

  prepareJumpToPresent(): void {
    this.cancelBottomScroll();
    this.followBottom();
    this.initialScrollDone = false;
    this.unlockScrollUp();
  }

  markUserScrollIntent(now = Date.now()): void {
    this.#userScrollIntentAt = now;
    this.#intentRevision += 1;
    this.cancelBottomScroll();
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

    if (!observation.alwaysScrollToBottom) {
      const scrollUpLocked = observation.now < this.#scrollUpLockedUntil;
      if (distanceFromBottom < 10 && !scrollUpLocked) {
        const wasScrolledUp = !this.shouldScrollToBottom;
        this.followBottom();
        reachedBottom =
          wasScrolledUp &&
          observation.now - this.#userScrollIntentAt < USER_SCROLL_INTENT_MS;
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
    }

    this.#previousOffset = observation.offset;
    if (!this.shouldScrollToBottom && observation.firstVisibleAt) {
      this.firstVisibleAt = observation.firstVisibleAt;
    }

    return { distanceFromBottom, reachedBottom };
  }

  reconcileAfterTabResume(
    distanceFromBottom: number,
    alwaysScrollToBottom: boolean
  ): void {
    if (alwaysScrollToBottom || !this.shouldScrollToBottom || !this.initialScrollDone) return;
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
    isJumpedMode: boolean,
    alwaysScrollToBottom: boolean
  ): boolean {
    return (
      token.operationId === this.#bottomScrollOperation &&
      token.roomId === currentRoomId &&
      token.intentRevision === this.#intentRevision &&
      !isJumpedMode &&
      (alwaysScrollToBottom || this.shouldScrollToBottom)
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
