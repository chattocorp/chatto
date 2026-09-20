import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { appState } from '$lib/state/globals.svelte';
import { ReadViewRegistry } from './readViews.svelte';

describe('ReadViewRegistry', () => {
  let wasFocused: boolean;
  let wasVisible: boolean;
  beforeEach(() => {
    wasFocused = appState.isFocused;
    wasVisible = appState.isVisible;
    appState.isFocused = true;
    appState.isVisible = true;
  });
  afterEach(() => {
    appState.isFocused = wasFocused;
    appState.isVisible = wasVisible;
  });

  it('covers independent views and releases duplicate views separately', () => {
    const views = new ReadViewRegistry();
    const target = { roomId: 'room', threadRootId: 'a' };
    const closeFirst = views.register(target);
    const closeDuplicate = views.register(target);
    views.register({ roomId: 'room', threadRootId: 'b' });
    views.register({ roomId: 'other-room' });
    closeFirst();
    closeFirst();
    expect(views.covers('room', 'a')).toBe(true);
    closeDuplicate();
    expect(views.covers('room', 'a')).toBe(false);
    expect(views.covers('room', 'b')).toBe(true);
    expect(views.covers('room')).toBe(false);
    expect(views.covers('other-room')).toBe(true);
    expect(views.covers('other-room', 'b')).toBe(false);
    expect(new ReadViewRegistry().covers('room', 'b')).toBe(false);
  });

  it('requires focus and visibility and discards registrations on clear', () => {
    const views = new ReadViewRegistry();
    const close = views.register({ roomId: 'room', threadRootId: 'a' });
    appState.isFocused = false;
    expect(views.covers('room', 'a')).toBe(false);
    appState.isFocused = true;
    appState.isVisible = false;
    expect(views.covers('room', 'a')).toBe(false);
    appState.isVisible = true;
    expect(views.covers('room', 'a')).toBe(true);
    views.clear();
    views.register({ roomId: 'room', threadRootId: 'a' });
    close();
    expect(views.covers('room', 'a')).toBe(true);
  });
});
