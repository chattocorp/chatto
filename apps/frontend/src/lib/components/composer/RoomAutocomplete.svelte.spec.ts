import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import type { RoomListItem } from '@chatto/client/server/rooms';
import RoomAutocomplete from './RoomAutocomplete.svelte';

function room(id: string, name: string, type = RoomKind.CHANNEL): RoomListItem {
  return {
    id,
    name,
    type,
    isUniversal: false,
    viewerIsMember: false,
    viewerCanJoinRoom: true,
    viewerCanManageRoom: false,
    members: []
  };
}

describe('RoomAutocomplete', () => {
  it('finds names with spaces through fuzzy search and accepts them with Enter', () => {
    const onSelect = vi.fn();
    const { container, component } = render(RoomAutocomplete, {
      query: 'designteam',
      rooms: [room('1', 'Design Team'), room('2', 'Other room')],
      onSelect,
      onClose: () => {}
    });
    expect(container.querySelectorAll('button')).toHaveLength(1);
    expect(container.textContent).toContain('Design Team');
    const event = new KeyboardEvent('keydown', { key: 'Enter', cancelable: true });
    expect(component.handleKeyDown(event)).toBe(true);
    expect(event.defaultPrevented).toBe(true);
    expect(onSelect).toHaveBeenCalledWith('1', 'Enter');
  });

  it('shows a globe for universal rooms and a hash for ordinary channels', () => {
    const { container } = render(RoomAutocomplete, {
      query: '',
      rooms: [{ ...room('1', 'announcements'), isUniversal: true }, room('2', 'general')],
      onSelect: () => {},
      onClose: () => {}
    });
    const buttons = container.querySelectorAll('button');
    expect(buttons[0].querySelector('[class~="icon-[uil--globe]"]')).not.toBeNull();
    expect(buttons[0].textContent).not.toContain('#');
    expect(buttons[1].textContent).toContain('#');
  });
  it('filters channel names, excludes DMs, and selects with the keyboard', () => {
    const onSelect = vi.fn();
    const onClose = vi.fn();
    const { container, component } = render(RoomAutocomplete, {
      query: 'gen',
      rooms: [
        room('2', 'general'),
        room('1', 'gen'),
        room('3', 'random'),
        room('4', 'gen DM', RoomKind.DM)
      ],
      onSelect,
      onClose
    });
    expect(
      Array.from(container.querySelectorAll('button')).map((button) => button.textContent?.trim())
    ).toEqual(['# gen', '# general']);
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
    flushSync();
    const enter = new KeyboardEvent('keydown', { key: 'Enter', cancelable: true });
    expect(component.handleKeyDown(enter)).toBe(true);
    expect(enter.defaultPrevented).toBe(true);
    expect(onSelect).toHaveBeenCalledWith('2', 'Enter');
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('supports bare #, Tab and pointer selection, and updates when rooms change', async () => {
    const onSelect = vi.fn();
    const { container, component, rerender } = render(RoomAutocomplete, {
      query: '',
      rooms: [room('1', '日本語'), room('2', 'general')],
      onSelect,
      onClose: () => {}
    });
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Tab' }));
    expect(onSelect).toHaveBeenCalledWith('2', 'Tab');
    container.querySelectorAll('button')[1].click();
    expect(onSelect).toHaveBeenCalledWith('1', 'click');
    await rerender({ query: '日本' });
    expect(container.querySelectorAll('button')).toHaveLength(1);
    await rerender({ rooms: [] });
    expect(container.querySelector('[data-testid="room-autocomplete"]')).toBeNull();
    expect(component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Enter' }))).toBe(false);
  });
});
