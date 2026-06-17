import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import MentionAutocomplete from './MentionAutocomplete.svelte';
import type { RoomMember } from '$lib/state/room';
import { PresenceStatus } from '$lib/gql/graphql';

function member(login: string, displayName?: string): RoomMember {
  return {
    id: `u_${login}`,
    login,
    displayName: displayName ?? login,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Offline
  };
}

function renderAutocomplete(props: {
  query: string;
  members: RoomMember[];
  roles?: { name: string; isSystem?: boolean; position?: number; pingable?: boolean }[];
  onSelect?: (login: string, viaTab: boolean) => void;
  onClose?: () => void;
}) {
  return render(MentionAutocomplete, {
    props: {
      query: props.query,
      members: props.members,
      roles: props.roles ?? [],
      onSelect: props.onSelect ?? (() => {}),
      onClose: props.onClose ?? (() => {})
    }
  });
}

function visibleLogins(container: HTMLElement): string[] {
  // The snippet renders @login as a `<span class="text-muted">@login</span>`
  return Array.from(container.querySelectorAll('span'))
    .map((s) => s.textContent ?? '')
    .filter((t) => t.startsWith('@'))
    .map((t) => t.slice(1));
}

function activeLogin(container: HTMLElement): string | null {
  const active = container.querySelector('.menu-item-active');
  if (!active) return null;
  const span = Array.from(active.querySelectorAll('span'))
    .map((s) => s.textContent ?? '')
    .find((t) => t.startsWith('@'));
  return span ? span.slice(1) : null;
}

describe('MentionAutocomplete', () => {
  describe('filtering', () => {
    it('renders nothing when no members match', () => {
      const { container } = renderAutocomplete({
        query: 'zzznomatch',
        members: [member('alice'), member('bob')]
      });
      expect(container.querySelector('[data-testid="mention-autocomplete"]')).toBeNull();
    });

    it('renders members whose login fuzzy-matches the query', () => {
      const { container } = renderAutocomplete({
        query: 'ali',
        members: [member('alice'), member('bob'), member('charlie')]
      });
      expect(visibleLogins(container)).toContain('alice');
      expect(visibleLogins(container)).not.toContain('bob');
    });

    it('matches on display name as well as login', () => {
      const { container } = renderAutocomplete({
        query: 'wonder',
        members: [member('alice', 'Alice Wonderland'), member('bob', 'Bob Smith')]
      });
      expect(visibleLogins(container)).toEqual(['alice']);
    });

    it('limits results to the top 10', () => {
      const members = Array.from({ length: 20 }, (_, i) => member('user' + i, 'User ' + i));
      const { container } = renderAutocomplete({ query: 'user', members });
      expect(visibleLogins(container).length).toBe(10);
    });

    it('orders results by descending fuzzy score (exact match first)', () => {
      const { container } = renderAutocomplete({
        query: 'al',
        members: [member('alistair'), member('al'), member('aldous')]
      });
      const order = visibleLogins(container);
      expect(order[0]).toBe('al'); // exact match wins
    });

    it('includes virtual mention handles and pingable role names', () => {
      const { container } = renderAutocomplete({
        query: 'he',
        members: [],
        roles: [
          { name: 'helpdesk', pingable: true },
          { name: 'helpdesk-quiet', pingable: false },
          { name: 'everyone', pingable: true }
        ]
      });
      const order = visibleLogins(container);
      expect(order).toContain('here');
      expect(order).toContain('helpdesk');
      expect(order).not.toContain('helpdesk-quiet');
      expect(order).not.toContain('everyone');
    });

    it('ranks by max(loginScore, displayScore): a strong displayName match beats a weak login match', () => {
      // m1's login contains "alpha" deep in the string (low score), but its
      // displayName is an *exact* match (highest possible score).
      // m2's login is a prefix match (decent score), displayName doesn't match.
      // If the component used only loginScore, m2 would rank above m1.
      // With Math.max(login, display), m1 must come first.
      const m1 = member('zzz_alpha_user', 'alpha');
      const m2 = member('alphaa', 'zzz');
      const { container } = renderAutocomplete({
        query: 'alpha',
        members: [m2, m1]
      });
      const order = visibleLogins(container);
      expect(order[0]).toBe('zzz_alpha_user');
      expect(order[1]).toBe('alphaa');
    });
  });

  describe('keyboard forwarding', () => {
    it('forwards keydown to the underlying popup and reports it as handled', () => {
      const { container, component } = renderAutocomplete({
        query: 'a',
        members: [member('alice'), member('amos')]
      });
      const ev = new KeyboardEvent('keydown', { key: 'ArrowDown', cancelable: true });
      const handled = component.handleKeyDown(ev);
      flushSync();
      expect(handled).toBe(true);
      expect(activeLogin(container)).not.toBeNull();
    });

    it('returns false when there are no items to navigate', () => {
      const { component } = renderAutocomplete({
        query: 'zzz',
        members: [member('alice')]
      });
      const ev = new KeyboardEvent('keydown', { key: 'ArrowDown', cancelable: true });
      expect(component.handleKeyDown(ev)).toBe(false);
    });

    it('Tab triggers onSelect with viaTab=true', () => {
      const onSelect = vi.fn();
      const { component } = renderAutocomplete({
        query: 'al',
        members: [member('alice')],
        onSelect
      });
      component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Tab', cancelable: true }));
      expect(onSelect).toHaveBeenCalledWith('alice', true);
    });

    it('Enter selects the highlighted mention with viaTab=false', () => {
      const onSelect = vi.fn();
      const { component } = renderAutocomplete({
        query: 'al',
        members: [member('alice')],
        onSelect
      });
      const ev = new KeyboardEvent('keydown', { key: 'Enter', cancelable: true });
      const handled = component.handleKeyDown(ev);
      expect(handled).toBe(true);
      expect(ev.defaultPrevented).toBe(true);
      expect(onSelect).toHaveBeenCalledWith('alice', false);
    });

    it('Escape calls onClose', () => {
      const onClose = vi.fn();
      const { component } = renderAutocomplete({
        query: 'al',
        members: [member('alice')],
        onClose
      });
      component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));
      expect(onClose).toHaveBeenCalledOnce();
    });
  });

  describe('selection', () => {
    it('clicking a result calls onSelect with viaTab=false', () => {
      const onSelect = vi.fn();
      const { container } = renderAutocomplete({
        query: 'al',
        members: [member('alice'), member('aldous')],
        onSelect
      });
      const buttons = container.querySelectorAll('button');
      (buttons[0] as HTMLButtonElement).click();
      // Click sends the literal 'click' key, which !== 'Tab' → viaTab=false
      expect(onSelect).toHaveBeenCalledTimes(1);
      const [, viaTab] = onSelect.mock.calls[0];
      expect(viaTab).toBe(false);
    });
  });
});
