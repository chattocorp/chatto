import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';

import { q } from '$lib/test-utils';
import MenuItemTestHarness from './MenuItemTestHarness.svelte';

let originalShowPopover: typeof HTMLElement.prototype.showPopover;
let originalShowModal: typeof HTMLDialogElement.prototype.showModal;

beforeAll(() => {
  originalShowPopover = HTMLElement.prototype.showPopover;
  originalShowModal = HTMLDialogElement.prototype.showModal;
  HTMLElement.prototype.showPopover = function showPopover() {
    this.setAttribute('popover-open', '');
  };
  HTMLDialogElement.prototype.showModal = function showModal() {
    this.setAttribute('open', '');
  };
});

afterAll(() => {
  HTMLElement.prototype.showPopover = originalShowPopover;
  HTMLDialogElement.prototype.showModal = originalShowModal;
});

describe('MenuItem', () => {
  it('renders button, link, icon, and text-only entries with menu semantics', () => {
    const { container } = render(MenuItemTestHarness);
    const iconButton = q(container, '[data-testid="icon-button"]') as HTMLButtonElement;
    const textButton = q(container, '[data-testid="text-button"]') as HTMLButtonElement;
    const link = q(container, '[data-testid="link-item"]') as HTMLAnchorElement;

    expect(iconButton.type).toBe('button');
    expect(iconButton.getAttribute('role')).toBe('menuitem');
    expect(
      Array.from(iconButton.querySelectorAll('.iconify')).some((icon) =>
        icon.classList.contains('icon-[uil--copy]')
      )
    ).toBe(true);
    expect(iconButton.querySelector('.menu-entry-leading')?.classList).toContain('self-start');
    expect(iconButton.querySelector('.menu-entry-leading')?.classList).toContain(
      'menu-entry-leading-floating'
    );
    expect(textButton.querySelector('.menu-entry-leading')).toBeNull();
    expect(link.tagName).toBe('A');
    expect(link.getAttribute('href')).toBe('/settings');
    expect(link.getAttribute('role')).toBe('menuitem');
  });

  it('calls available actions and blocks disabled buttons and links', () => {
    const { container } = render(MenuItemTestHarness);
    const iconButton = q(container, '[data-testid="icon-button"]') as HTMLButtonElement;
    const disabledButton = q(container, '[data-testid="disabled-button"]') as HTMLButtonElement;
    const disabledLink = q(container, '[data-testid="disabled-link"]') as HTMLAnchorElement;
    const clickCount = q(container, '[data-testid="click-count"]');

    iconButton.click();
    flushSync();
    expect(clickCount?.textContent).toBe('1');

    disabledButton.click();
    flushSync();
    expect(disabledButton.disabled).toBe(true);
    expect(clickCount?.textContent).toBe('1');

    const event = new MouseEvent('click', { bubbles: true, cancelable: true });
    disabledLink.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    expect(disabledLink.getAttribute('aria-disabled')).toBe('true');
    expect(disabledLink.tabIndex).toBe(-1);
    expect(clickCount?.textContent).toBe('1');
  });

  it('renders custom leading and trailing content with checked state', () => {
    const { container } = render(MenuItemTestHarness);
    const customItem = q(container, '[data-testid="custom-item"]') as HTMLButtonElement;

    expect(customItem.getAttribute('role')).toBe('menuitemradio');
    expect(customItem.getAttribute('aria-checked')).toBe('true');
    expect(customItem.classList).toContain('menu-entry-selected');
    expect(customItem.querySelector('.menu-entry-leading')?.textContent).toContain('🙂');
    expect(
      Array.from(customItem.querySelectorAll('.iconify')).some((icon) =>
        icon.classList.contains('icon-[uil--check]')
      )
    ).toBe(true);
  });

  it('uses touch-safe density in the sheet presentation', () => {
    const { container } = render(MenuItemTestHarness, {
      props: { presentation: 'sheet' }
    });
    const item = q(container, '[data-testid="icon-button"]');

    expect(item?.classList).toContain('menu-entry-sheet');
    expect(item?.querySelector('.menu-entry-leading')?.classList).toContain(
      'menu-entry-leading-sheet'
    );
    expect(item?.querySelector('.menu-entry-leading')?.classList).not.toContain(
      'menu-entry-leading-floating'
    );
  });

  it('does not add menu roles inside a dialog-style context menu', () => {
    const { container } = render(MenuItemTestHarness, {
      props: { containerRole: 'dialog' }
    });

    expect(q(container, '[data-testid="icon-button"]')?.getAttribute('role')).toBeNull();
  });
});
