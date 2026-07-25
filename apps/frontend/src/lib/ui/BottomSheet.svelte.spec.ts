import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { testSnippet } from '$lib/test-utils';
import BottomSheet from './BottomSheet.svelte';

let originalShowModal: typeof HTMLDialogElement.prototype.showModal;
let originalClose: typeof HTMLDialogElement.prototype.close;

function renderSheet(onclose = vi.fn()) {
  return render(BottomSheet, {
    props: {
      visible: true,
      onclose,
      children: testSnippet(
        '<div><input type="text" aria-label="Search"><button>Choose</button></div>'
      )
    }
  });
}

function sheetElements(container: HTMLElement) {
  const dialog = container.querySelector<HTMLDialogElement>('dialog');
  const input = container.querySelector<HTMLInputElement>('input');
  if (!dialog || !input) throw new Error('BottomSheet test fixture did not render');
  return { dialog, input };
}

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

afterAll(() => {
  HTMLDialogElement.prototype.showModal = originalShowModal;
  HTMLDialogElement.prototype.close = originalClose;
});

describe('BottomSheet', () => {
  it('does not dismiss from a click inside content without a preceding pointer event', () => {
    const { container } = renderSheet();
    const { dialog, input } = sheetElements(container);

    input.click();

    expect(dialog).not.toHaveClass('closing');
    expect(dialog).toHaveAttribute('open');
  });

  it('ignores a keyboard cancel that arrives while focus is transferring to an input', () => {
    const { container } = renderSheet();
    const { dialog, input } = sheetElements(container);

    input.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, pointerType: 'touch' }));
    dialog.dispatchEvent(new Event('cancel', { cancelable: true }));

    expect(dialog).not.toHaveClass('closing');
    expect(dialog).toHaveAttribute('open');
  });

  it('ignores the keyboard cancel when a touch browser omits pointer events', () => {
    const { container } = renderSheet();
    const { dialog, input } = sheetElements(container);

    input.dispatchEvent(new TouchEvent('touchstart', { bubbles: true }));
    dialog.dispatchEvent(new Event('cancel', { cancelable: true }));

    expect(dialog).not.toHaveClass('closing');
    expect(dialog).toHaveAttribute('open');
  });

  it('stops guarding cancel after editable focus has completed and moved away', () => {
    const { container } = renderSheet();
    const { dialog, input } = sheetElements(container);

    input.dispatchEvent(new TouchEvent('touchstart', { bubbles: true }));
    input.focus();
    input.blur();
    dialog.dispatchEvent(new Event('cancel', { cancelable: true }));
    flushSync();

    expect(dialog).toHaveClass('closing');
  });

  it('dismisses directly from a backdrop pointerdown', () => {
    const { container } = renderSheet();
    const { dialog } = sheetElements(container);

    dialog.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, pointerType: 'touch' }));
    flushSync();

    expect(dialog).toHaveClass('closing');
  });

  it('dismisses a cancel request when focus is not in an editable control', () => {
    const { container } = renderSheet();
    const { dialog } = sheetElements(container);

    dialog.dispatchEvent(new Event('cancel', { cancelable: true }));
    flushSync();

    expect(dialog).toHaveClass('closing');
  });
});
