import { render } from 'vitest-browser-svelte';
import { describe, expect, it } from 'vitest';
import { q, testSnippet } from '$lib/test-utils';
import CollapsibleGroup from './CollapsibleGroup.svelte';

describe('CollapsibleGroup', () => {
  it('mirrors its collapsed inline-end disclosure icon in RTL', () => {
    const { container } = render(CollapsibleGroup, {
      props: {
        label: 'Rooms',
        items: [{ id: 'room-1' }],
        item: testSnippet('<span>General</span>'),
        persistKey: 'test:collapsible-group:rtl',
        defaultCollapsed: true
      }
    });

    const icon = q(container, '.iconify');
    expect(icon?.classList).toContain('icon-[uil--angle-right-b]');
    expect(icon?.classList).toContain('rtl:-scale-x-100');
    expect(icon?.classList).not.toContain('rotate-90');
  });
});
