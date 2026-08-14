import { render } from 'vitest-browser-svelte';
import { describe, expect, it } from 'vitest';
import { q, testSnippet } from '$lib/test-utils';
import CollapsibleGroupStack from './CollapsibleGroupStack.svelte';

describe('CollapsibleGroupStack', () => {
  it('separates adjacent groups and lets each group collapse independently', async () => {
    const { container } = render(CollapsibleGroupStack, {
      props: {
        groups: [
          {
            id: 'online',
            label: 'Online',
            items: [{ id: 'online-user' }],
            persistKey: 'test:collapsible-group-stack:online'
          },
          {
            id: 'offline',
            label: 'Offline',
            items: [{ id: 'offline-user' }],
            persistKey: 'test:collapsible-group-stack:offline'
          }
        ],
        item: testSnippet('<span>User</span>')
      }
    });

    const toggles = container.querySelectorAll<HTMLButtonElement>('button[aria-expanded]');
    const separator = q(container, '[data-testid="collapsible-group-separator"]');

    expect(toggles).toHaveLength(2);
    expect(separator?.previousElementSibling?.contains(toggles[0])).toBe(true);
    expect(separator?.nextElementSibling?.contains(toggles[1])).toBe(true);

    toggles[1].click();
    await expect.element(toggles[0]).toHaveAttribute('aria-expanded', 'true');
    await expect.element(toggles[1]).toHaveAttribute('aria-expanded', 'false');

    toggles[1].click();
    await expect.element(toggles[1]).toHaveAttribute('aria-expanded', 'true');
  });

  it('does not render a separator for one group', () => {
    const { container } = render(CollapsibleGroupStack, {
      props: {
        groups: [
          {
            id: 'members',
            label: 'Members',
            items: [{ id: 'user' }],
            persistKey: 'test:collapsible-group-stack:single'
          }
        ],
        item: testSnippet('<span>User</span>')
      }
    });

    expect(container.querySelector('[data-testid="collapsible-group-separator"]')).toBeNull();
  });
});
