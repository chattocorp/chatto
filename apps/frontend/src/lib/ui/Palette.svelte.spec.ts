import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { sidebarNav } from '$lib/state/globals.svelte';
import { testSnippet } from '$lib/test-utils';
import Palette from './Palette.svelte';

let originalShowModal: typeof HTMLDialogElement.prototype.showModal;
let originalClose: typeof HTMLDialogElement.prototype.close;

const children = testSnippet(
  '<div class="menu-section"><button type="button">Palette item</button></div>'
);

beforeAll(() => {
  originalShowModal = HTMLDialogElement.prototype.showModal;
  originalClose = HTMLDialogElement.prototype.close;
  HTMLDialogElement.prototype.showModal = function showModal() {
    this.setAttribute('open', '');
  };
  HTMLDialogElement.prototype.close = function close() {
    this.removeAttribute('open');
    this.dispatchEvent(new Event('close'));
  };
});

beforeEach(() => sidebarNav.setMobile(false));
afterEach(() => sidebarNav.setMobile(false));

afterAll(() => {
  HTMLDialogElement.prototype.showModal = originalShowModal;
  HTMLDialogElement.prototype.close = originalClose;
});

describe('Palette', () => {
  it('owns the exact shared shell for a modal palette', () => {
    const onclose = vi.fn();
    const { container } = render(Palette, {
      props: {
        id: 'palette-test',
        visible: true,
        ariaLabel: 'Test palette',
        onclose,
        children
      }
    });

    const dialog = container.querySelector<HTMLDialogElement>('dialog.palette-dialog');
    const shell = container.querySelector<HTMLElement>('#palette-test');
    expect(dialog).toHaveAttribute('open');
    expect(dialog).toHaveAttribute('aria-label', 'Test palette');
    expect(shell).toHaveClass('menu', 'w-140', 'max-w-[90vw]', 'gap-1');

    dialog!.dispatchEvent(new Event('cancel', { cancelable: true }));
    expect(onclose).toHaveBeenCalledOnce();
  });

  it('uses that same shell in an anchored popover', () => {
    const { container } = render(Palette, {
      props: {
        id: 'palette-test',
        visible: true,
        presentation: 'anchored',
        anchor: { top: 20, bottom: 60, left: 20 },
        ariaLabel: 'Test palette',
        onclose: vi.fn(),
        children
      }
    });

    const popover = container.querySelector<HTMLElement>('[popover="manual"]');
    const shell = container.querySelector<HTMLElement>('#palette-test');
    expect(popover).not.toBeNull();
    expect(popover).toHaveAttribute('role', 'dialog');
    expect(shell).toHaveClass('menu', 'w-140', 'max-w-[90vw]', 'gap-1');
  });

  it('uses that same shell in the mobile bottom sheet', () => {
    sidebarNav.setMobile(true);
    flushSync();

    const { container } = render(Palette, {
      props: {
        id: 'palette-test',
        visible: true,
        presentation: 'anchored',
        anchor: { top: 20, bottom: 60, left: 20 },
        ariaLabel: 'Test palette',
        onclose: vi.fn(),
        children
      }
    });

    expect(container.querySelector('dialog.bottom-sheet')).toHaveAttribute('open');
    expect(container.querySelector('#palette-test')).toHaveClass(
      'menu',
      'w-140',
      'max-w-[90vw]',
      'gap-1'
    );
  });

  it('keeps one logical session while switching responsive presentations', async () => {
    const onopen = vi.fn();
    const onclosed = vi.fn();
    const rendered = render(Palette, {
      props: {
        id: 'palette-test',
        visible: true,
        ariaLabel: 'Test palette',
        onclose: vi.fn(),
        onopen,
        onclosed,
        children
      }
    });

    expect(onopen).toHaveBeenCalledOnce();
    expect(onclosed).not.toHaveBeenCalled();

    sidebarNav.setMobile(true);
    flushSync();

    expect(rendered.container.querySelector('dialog.bottom-sheet')).toHaveAttribute('open');
    expect(onopen).toHaveBeenCalledOnce();
    expect(onclosed).not.toHaveBeenCalled();

    await rendered.rerender({
      id: 'palette-test',
      visible: false,
      ariaLabel: 'Test palette',
      onclose: vi.fn(),
      onopen,
      onclosed,
      children
    });

    expect(onopen).toHaveBeenCalledOnce();
    expect(onclosed).toHaveBeenCalledOnce();
  });
});
