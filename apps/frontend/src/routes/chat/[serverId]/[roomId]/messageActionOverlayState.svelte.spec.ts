import { describe, expect, it } from 'vitest';
import { MessageActionOverlayState } from './messageActionOverlayState.svelte';

describe('MessageActionOverlayState', () => {
  it('opens one overlay for one message at a time', () => {
    const state = new MessageActionOverlayState();

    state.open('a', { kind: 'menu', position: { x: 1, y: 2 } }, { linkUrl: 'https://a.test' });
    expect(state.isOpenFor('a')).toBe(true);
    expect(state.keepsToolbarVisibleFor('a')).toBe(true);
    expect(state.linkUrl).toBe('https://a.test');

    state.open('b', { kind: 'sheet' });
    expect(state.isOpenFor('a')).toBe(false);
    expect(state.isOpenFor('b')).toBe(true);
    expect(state.keepsToolbarVisibleFor('b')).toBe(false);
    expect(state.linkUrl).toBeNull();
  });

  it('closes only an overlay of the given kind', () => {
    const state = new MessageActionOverlayState();

    // A menu opens the emoji picker and then closes itself.
    state.open('a', { kind: 'menu', position: { x: 1, y: 2 } });
    state.open('a', { kind: 'emoji', position: { x: 1, y: 2 }, presentation: 'auto' });
    state.close({ kind: 'menu' });
    expect(state.overlay?.kind).toBe('emoji');

    state.close({ kind: 'emoji' });
    expect(state.overlay).toBeNull();
    expect(state.eventId).toBeNull();
  });

  it('ignores a late close from another message', () => {
    const state = new MessageActionOverlayState();

    state.open('b', { kind: 'menu', position: { x: 1, y: 2 } });
    state.close({ kind: 'menu', eventId: 'a' });
    expect(state.isOpenFor('b')).toBe(true);
  });

  it('hands the reply quote to one reply only', () => {
    const state = new MessageActionOverlayState();

    state.open('a', { kind: 'sheet' }, { replyQuote: 'Quoted' });
    expect(state.takeReplyQuote()).toBe('Quoted');
    expect(state.takeReplyQuote()).toBeNull();
  });
});
