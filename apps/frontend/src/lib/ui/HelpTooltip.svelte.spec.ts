import { describe, it, expect } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import HelpTooltip from './HelpTooltip.svelte';
import { testSnippet } from '$lib/test-utils';

function setup() {
  const { container } = render(HelpTooltip, {
    props: {
      label: 'Show help',
      children: testSnippet('<span>Help body text</span>')
    }
  });
  const trigger = container.querySelector('button[aria-label]') as HTMLButtonElement;
  if (!trigger) throw new Error('trigger button not rendered');
  const popover = () => container.querySelector('[role="tooltip"]');
  return { container, trigger, popover };
}

describe('HelpTooltip', () => {
  it('renders the trigger with the provided label and is closed by default', () => {
    const { trigger, popover } = setup();
    expect(trigger.getAttribute('aria-label')).toBe('Show help');
    expect(trigger.getAttribute('aria-describedby')).toBeNull();
    expect(popover()).toBeNull();
  });

  it('wires aria-describedby to the tooltip element when open', () => {
    const { trigger, popover } = setup();

    trigger.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();
    const describedBy = trigger.getAttribute('aria-describedby');
    expect(describedBy).toBeTruthy();
    expect(popover()?.id).toBe(describedBy);

    trigger.dispatchEvent(new MouseEvent('mouseleave'));
    flushSync();
    expect(trigger.getAttribute('aria-describedby')).toBeNull();
  });

  it('shows the popover transiently on mouseenter and hides on mouseleave', () => {
    const { trigger, popover } = setup();

    trigger.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();
    expect(popover()?.textContent?.trim()).toBe('Help body text');

    trigger.dispatchEvent(new MouseEvent('mouseleave'));
    flushSync();
    expect(popover()).toBeNull();
  });

  it('shows on focus and hides on blur', () => {
    const { trigger, popover } = setup();

    trigger.dispatchEvent(new FocusEvent('focus'));
    flushSync();
    expect(popover()).not.toBeNull();

    trigger.dispatchEvent(new FocusEvent('blur'));
    flushSync();
    expect(popover()).toBeNull();
  });

  it('pins the popover open when the trigger is clicked, ignoring mouseleave', () => {
    const { trigger, popover } = setup();

    trigger.click();
    flushSync();
    expect(popover()).not.toBeNull();

    // Once pinned, mouseleave must not close it.
    trigger.dispatchEvent(new MouseEvent('mouseleave'));
    flushSync();
    expect(popover()).not.toBeNull();
  });

  it('a second click unpins and closes the popover', () => {
    const { trigger, popover } = setup();

    trigger.click();
    flushSync();
    expect(popover()).not.toBeNull();

    trigger.click();
    flushSync();
    expect(popover()).toBeNull();
  });

  it('a pointerdown outside the wrapper dismisses a pinned popover', async () => {
    const { trigger, popover } = setup();

    trigger.click();
    flushSync();
    expect(popover()).not.toBeNull();

    // FloatingPopover defers installing its pointerdown listener by one
    // animation frame so the opening click doesn't immediately close it.
    await new Promise((r) => requestAnimationFrame(() => r(null)));
    document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
    flushSync();
    expect(popover()).toBeNull();
  });

  it('Escape dismisses a pinned popover', () => {
    const { trigger, popover } = setup();

    trigger.click();
    flushSync();
    expect(popover()).not.toBeNull();

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    flushSync();
    expect(popover()).toBeNull();
  });
});
