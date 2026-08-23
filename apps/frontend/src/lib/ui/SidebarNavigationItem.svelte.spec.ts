import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it, vi } from 'vitest';
import { q, testSnippet } from '$lib/test-utils';
import '../../app.css';
import SidebarNavigationItem from './SidebarNavigationItem.svelte';

function renderItem(options: { withStatus?: boolean } = {}) {
  return render(SidebarNavigationItem, {
    props: {
      href: '/chat/-/dm-1',
      children: testSnippet('<span>River</span>'),
      status: options.withStatus
        ? testSnippet('<button type="button" data-testid="status">3 notifications</button>')
        : undefined,
      hoverAction: testSnippet(
        '<button type="button" data-testid="hover-action">Archive conversation</button>'
      ),
      testid: 'sidebar-row'
    }
  });
}

describe('SidebarNavigationItem', () => {
  it('renders the link and trailing controls as siblings', () => {
    const { container } = renderItem({ withStatus: true });
    const row = q(container, '[data-testid="sidebar-row"]');
    const link = q(row!, 'a');
    const status = q(row!, '[data-testid="status"]');
    const action = q(row!, '[data-testid="hover-action"]');

    expect(link?.parentElement).toBe(row);
    expect(status?.closest('a')).toBeNull();
    expect(action?.closest('a')).toBeNull();
  });

  it('replaces status with the action while the row is hovered', async () => {
    const { container } = renderItem({ withStatus: true });
    const row = q(container, '[data-testid="sidebar-row"]') as HTMLElement;
    const status = q(row, '[data-sidebar-status]') as HTMLElement;
    const action = q(row, '[data-sidebar-hover-action]') as HTMLElement;

    await userEvent.unhover(row);
    await vi.waitFor(() => expect(getComputedStyle(action).opacity).toBe('0'));
    expect(getComputedStyle(status).opacity).toBe('1');

    await userEvent.hover(row);

    await vi.waitFor(() => expect(getComputedStyle(status).opacity).toBe('0'));
    expect(getComputedStyle(status).pointerEvents).toBe('none');
    expect(getComputedStyle(action).opacity).toBe('1');
    expect(getComputedStyle(action).pointerEvents).toBe('auto');
  });

  it('reveals the action on keyboard focus and hides status', async () => {
    const { container } = renderItem({ withStatus: true });
    const row = q(container, '[data-testid="sidebar-row"]') as HTMLElement;
    const status = q(row, '[data-sidebar-status]') as HTMLElement;
    const actionWrapper = q(row, '[data-sidebar-hover-action]') as HTMLElement;
    const action = q(row, '[data-testid="hover-action"]') as HTMLButtonElement;

    action.focus();

    await vi.waitFor(() => {
      expect(getComputedStyle(actionWrapper).opacity).toBe('1');
      expect(getComputedStyle(status).opacity).toBe('0');
    });
    expect(document.activeElement).toBe(action);
  });

  it('reserves the trailing action slot when no status is present', async () => {
    const { container } = renderItem();
    const row = q(container, '[data-testid="sidebar-row"]') as HTMLElement;
    const action = q(row, '[data-sidebar-hover-action]') as HTMLElement;

    await userEvent.unhover(row);
    await vi.waitFor(() => expect(getComputedStyle(action).opacity).toBe('0'));
    expect(row.querySelector('[data-sidebar-status]')).toBeNull();
    expect(action.parentElement?.classList).toContain('min-w-6');
  });
});
