import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import ContextMenu from './ContextMenu.svelte';

const inputCapabilities = vi.hoisted(() => ({
  prefersTouchActions: vi.fn(() => false),
  supportsHoverActions: vi.fn(() => true)
}));

vi.mock('$lib/utils/inputCapabilities', () => inputCapabilities);

let originalShowPopover: typeof HTMLElement.prototype.showPopover;
let originalHidePopover: typeof HTMLElement.prototype.hidePopover;
let originalShowModal: typeof HTMLDialogElement.prototype.showModal;
let originalClose: typeof HTMLDialogElement.prototype.close;

function renderMenu(props: Record<string, unknown> = {}) {
  return render(ContextMenu, {
    props: {
      position: { x: 24, y: 32 },
      onclose: vi.fn(),
      children: testSnippet('<span>Menu body</span>'),
      ...props
    }
  });
}

beforeAll(() => {
  originalShowPopover = HTMLElement.prototype.showPopover;
  originalHidePopover = HTMLElement.prototype.hidePopover;
  originalShowModal = HTMLDialogElement.prototype.showModal;
  originalClose = HTMLDialogElement.prototype.close;

  HTMLElement.prototype.showPopover = function showPopover() {
    this.setAttribute('popover-open', '');
  };
  HTMLElement.prototype.hidePopover = function hidePopover() {
    this.removeAttribute('popover-open');
  };
  HTMLDialogElement.prototype.showModal = function showModal() {
    this.setAttribute('open', '');
  };
  HTMLDialogElement.prototype.close = function close() {
    this.removeAttribute('open');
    this.dispatchEvent(new Event('close'));
  };
});

afterAll(() => {
  HTMLElement.prototype.showPopover = originalShowPopover;
  HTMLElement.prototype.hidePopover = originalHidePopover;
  HTMLDialogElement.prototype.showModal = originalShowModal;
  HTMLDialogElement.prototype.close = originalClose;
});

beforeEach(() => {
  vi.clearAllMocks();
  inputCapabilities.prefersTouchActions.mockReturnValue(false);
  inputCapabilities.supportsHoverActions.mockReturnValue(true);
});

describe('ContextMenu', () => {
  it('dismisses on outside scroll by default', async () => {
    const onclose = vi.fn();
    renderMenu({ onclose });

    await new Promise((resolve) => requestAnimationFrame(resolve));
    document.body.dispatchEvent(new Event('scroll'));

    expect(onclose).toHaveBeenCalledOnce();
  });

  it('can remain open during programmatic outside scroll', async () => {
    const onclose = vi.fn();
    renderMenu({ onclose, scrollDismissal: 'user' });

    await new Promise((resolve) => requestAnimationFrame(resolve));
    document.body.dispatchEvent(new Event('scroll'));

    expect(onclose).not.toHaveBeenCalled();
  });

  it('dismisses user-scroll mode on outside wheel input', async () => {
    const onclose = vi.fn();
    renderMenu({ onclose, scrollDismissal: 'user' });

    await new Promise((resolve) => requestAnimationFrame(resolve));
    document.body.dispatchEvent(new WheelEvent('wheel', { bubbles: true }));

    expect(onclose).toHaveBeenCalledOnce();
  });

  it('dismisses user-scroll mode when an outside touch scroll starts', async () => {
    const onclose = vi.fn();
    renderMenu({ onclose, scrollDismissal: 'user' });

    await new Promise((resolve) => requestAnimationFrame(resolve));
    document.body.dispatchEvent(
      new PointerEvent('pointerdown', { bubbles: true, pointerType: 'touch' })
    );

    expect(onclose).toHaveBeenCalledOnce();
  });

  it('keeps user-scroll mode open for wheel input inside the menu', async () => {
    const onclose = vi.fn();
    const { container } = renderMenu({ onclose, scrollDismissal: 'user' });

    await new Promise((resolve) => requestAnimationFrame(resolve));
    const menu = q(container, '[role="menu"]');
    if (!menu) throw new Error('menu not rendered');
    menu.dispatchEvent(new WheelEvent('wheel', { bubbles: true }));

    expect(onclose).not.toHaveBeenCalled();
  });

  it('uses floating presentation on hybrid devices by default', async () => {
    inputCapabilities.prefersTouchActions.mockReturnValue(true);
    inputCapabilities.supportsHoverActions.mockReturnValue(true);

    const { container } = renderMenu();

    await expect.element(q(container, '[role="menu"]')).toBeInTheDocument();
    expect(q(container, 'dialog')).toBeNull();
    expect(container.textContent).toContain('Menu body');
  });

  it('can force sheet presentation for touch-initiated nested menus', async () => {
    inputCapabilities.prefersTouchActions.mockReturnValue(true);
    inputCapabilities.supportsHoverActions.mockReturnValue(true);

    const { container } = renderMenu({ presentation: 'sheet' });

    await expect.element(q(container, 'dialog.bottom-sheet')).toBeInTheDocument();
    expect(q(container, '[role="menu"]')).toBeNull();
    expect(container.textContent).toContain('Menu body');
  });
});
