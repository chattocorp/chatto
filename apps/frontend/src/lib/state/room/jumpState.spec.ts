import { describe, expect, it, vi } from 'vitest';
import type {
  JumpResult,
  LoadNewerResult,
  MessagesStore
} from '@chatto/client/room/messages/MessagesStore';
import { JumpToMessageState } from './jumpState';

function fakeStore(options: {
  jump?: JumpResult;
  newer?: LoadNewerResult;
  loaded?: string[];
  canLoadNewer?: boolean;
}) {
  return {
    getEventById: (id: string) => ((options.loaded ?? []).includes(id) ? { id } : undefined),
    jumpToMessage: vi.fn(async () => options.jump ?? { status: 'shown' }),
    canLoadNewer: options.canLoadNewer ?? true,
    loadNewer: vi.fn(async (accept: () => boolean) => {
      accept();
      return options.newer ?? { status: 'loaded', hasNewer: true };
    }),
    clearAnchor: vi.fn(),
    setAnchor: vi.fn(() => true),
    jumpToLatest: vi.fn(async () => true)
  };
}

const asStore = (store: ReturnType<typeof fakeStore>) => store as unknown as MessagesStore;

describe('JumpToMessageState', () => {
  it('scrolls to a shown message and enters jumped mode after a replaced window', async () => {
    const state = new JumpToMessageState();
    await expect(state.show(asStore(fakeStore({ loaded: ['m1'] })), 'm1')).resolves.toBe(true);
    expect(state.scrollToEventId).toBe('m1');
    expect(state.isJumpedMode).toBe(false);

    const loaded = fakeStore({ jump: { status: 'loaded', hasNewer: true, hasOlder: true } });
    await expect(state.show(asStore(loaded), 'm2')).resolves.toBe(true);
    expect(state).toMatchObject({
      scrollToEventId: 'm2',
      isJumpedMode: true,
      hasReachedEnd: false,
      hasOlderMessages: true
    });
  });

  it('leaves jumped mode when the message is missing and keeps state when superseded', async () => {
    const state = new JumpToMessageState();
    state.isJumpedMode = true;
    state.scrollToEventId = 'previous';
    await expect(
      state.show(asStore(fakeStore({ jump: { status: 'superseded' } })), 'm2')
    ).resolves.toBe(false);
    expect(state.isJumpedMode).toBe(true);
    expect(state.scrollToEventId).toBe('previous');

    await expect(
      state.show(asStore(fakeStore({ jump: { status: 'missing' } })), 'm2')
    ).resolves.toBe(false);
    expect(state.isJumpedMode).toBe(false);
    expect(state.scrollToEventId).toBeNull();
  });

  it('releases the newer-page flag when a jump replaces the window', async () => {
    const state = new JumpToMessageState();
    state.isLoadingNewer = true;
    await state.show(asStore(fakeStore({ jump: { status: 'missing' } })), 'm2');
    expect(state.isLoadingNewer).toBe(false);
  });

  it('pages newer events while in jumped mode and ends at the latest event', async () => {
    const state = new JumpToMessageState();
    state.isJumpedMode = true;
    const store = fakeStore({ newer: { status: 'loaded', hasNewer: false } });
    await state.loadNewer(asStore(store));
    expect(state.hasReachedEnd).toBe(true);
    expect(state.isLoadingNewer).toBe(false);

    // The end is reached: no further page is read.
    await state.loadNewer(asStore(store));
    expect(store.loadNewer).toHaveBeenCalledOnce();
  });

  it('asks the store to drop a page after the user left jumped mode', async () => {
    const state = new JumpToMessageState();
    let accepted: boolean | undefined;
    const store = fakeStore({});
    store.loadNewer.mockImplementationOnce(async (accept) => {
      state.isJumpedMode = false;
      accepted = accept();
      return { status: 'declined' };
    });
    state.isJumpedMode = true;
    await state.loadNewer(asStore(store));
    expect(accepted).toBe(false);
    expect(state.isLoadingNewer).toBe(false);
    expect(state.hasReachedEnd).toBe(false);
  });

  it('leaves the flag to the replacement window when a page is superseded', async () => {
    const state = new JumpToMessageState();
    await state.loadNewer(asStore(fakeStore({ newer: { status: 'superseded' } })));
    expect(state.isLoadingNewer).toBe(true);
  });

  it('does not read without a newer cursor', async () => {
    const state = new JumpToMessageState();
    const store = fakeStore({ canLoadNewer: false });
    await state.loadNewer(asStore(store));
    expect(store.loadNewer).not.toHaveBeenCalled();
    expect(state.isLoadingNewer).toBe(false);
  });

  it('returns to the latest window and forgets the anchor', async () => {
    const state = new JumpToMessageState();
    state.isJumpedMode = true;
    const store = fakeStore({});
    await expect(state.returnToLatest(asStore(store))).resolves.toBe(true);
    expect(store.clearAnchor).toHaveBeenCalled();
    expect(store.jumpToLatest).toHaveBeenCalled();
    expect(state.isJumpedMode).toBe(false);
  });
});
