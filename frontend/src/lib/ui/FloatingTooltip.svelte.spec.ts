import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import FloatingTooltip from './FloatingTooltip.svelte';

describe('FloatingTooltip', () => {
  it('stays mounted when closed so measured parents do not get child-list mutations', () => {
    const { container } = render(FloatingTooltip, {
      props: {
        open: false,
        position: { x: 24, y: 32 },
        id: 'tooltip-closed',
        children: testSnippet('<span>Tooltip body</span>')
      }
    });

    const tooltip = container.querySelector('[role="tooltip"]') as HTMLElement;

    expect(tooltip).not.toBeNull();
    expect(tooltip.matches(':popover-open')).toBe(false);
    expect(getComputedStyle(tooltip).display).toBe('none');
  });

  it('renders short non-interactive tooltip content through the shared surface', async () => {
    const { container } = render(FloatingTooltip, {
      props: {
        position: { x: 24, y: 32 },
        id: 'tooltip-1',
        children: testSnippet('<span>Tooltip body</span>')
      }
    });

    const tooltip = container.querySelector('[role="tooltip"]') as HTMLElement;

    await expect.element(tooltip).toBeInTheDocument();
    expect(tooltip.id).toBe('tooltip-1');
    expect(tooltip.classList.contains('floating-tooltip')).toBe(true);
    expect(tooltip.textContent?.trim()).toBe('Tooltip body');
  });
});
