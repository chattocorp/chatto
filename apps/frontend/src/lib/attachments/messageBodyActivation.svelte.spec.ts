import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { messageBodyActivation } from './messageBodyActivation';

let body: HTMLDivElement;
let cleanup: void | (() => void);
const activate = vi.fn();

function click(target: Element = body, options: PointerEventInit = {}) {
  const init = {
    bubbles: true,
    cancelable: true,
    isPrimary: true,
    pointerType: 'mouse',
    detail: 1,
    ...options
  };
  target.dispatchEvent(new PointerEvent('pointerdown', init));
  target.dispatchEvent(new PointerEvent('pointerup', init));
  target.dispatchEvent(new PointerEvent('click', init));
}

beforeEach(() => {
  vi.useFakeTimers();
  activate.mockClear();
  body = document.createElement('div');
  body.innerHTML =
    '<p>Selectable message text</p><a href="#test"><strong>Link</strong></a><button>Time</button><span class="mention">Mention</span>';
  document.body.append(body);
  cleanup = messageBodyActivation(activate)(body);
});

afterEach(() => {
  cleanup?.();
  body.remove();
  window.getSelection()?.removeAllRanges();
  vi.useRealTimers();
});

it('activates mouse clicks and touch taps synchronously without a timer', () => {
  click();
  expect(activate).toHaveBeenCalledTimes(1);
  click(body, { pointerType: 'touch' });
  expect(activate).toHaveBeenCalledTimes(2);
});

it('allows body activation inside a keyboard-focusable timeline', () => {
  const timeline = document.createElement('div');
  timeline.tabIndex = 0;
  document.body.append(timeline);
  timeline.append(body);
  try {
    click(body.querySelector('p')!);
    expect(activate).toHaveBeenCalledTimes(1);
  } finally {
    timeline.remove();
  }
});

it('does not activate again for the second or third click', () => {
  click();
  click(body, { detail: 2 });
  click(body, { detail: 3 });
  expect(activate).toHaveBeenCalledTimes(1);
});

it('preserves nested controls, modified clicks, and prevented events', () => {
  for (const target of body.querySelectorAll('strong, button, .mention')) click(target);
  click(body, { ctrlKey: true });
  click(body, { shiftKey: true });
  click(body, { button: 2 });
  body.addEventListener('click', (event) => event.preventDefault(), { capture: true, once: true });
  click();
  vi.advanceTimersByTime(500);
  expect(activate).not.toHaveBeenCalled();
});

it('preserves selected text and cancels movement even without a final selection', () => {
  window.getSelection()?.selectAllChildren(body.querySelector('p')!);
  click();
  vi.advanceTimersByTime(500);
  expect(window.getSelection()?.toString()).toBe('Selectable message text');
  window.getSelection()?.removeAllRanges();
  body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, isPrimary: true }));
  window.dispatchEvent(new PointerEvent('pointermove', { clientX: 30 }));
  body.dispatchEvent(new PointerEvent('click', { bubbles: true, detail: 1 }));
  vi.advanceTimersByTime(500);
  expect(activate).not.toHaveBeenCalled();
});

it('opens when a normal click clears an old selection', () => {
  window.getSelection()?.selectAllChildren(body);
  body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, isPrimary: true }));
  window.getSelection()?.removeAllRanges();
  body.dispatchEvent(new PointerEvent('click', { bubbles: true, detail: 1 }));
  expect(activate).toHaveBeenCalledTimes(1);
});

it('checks selection after the browser click default action when needed', () => {
  window.getSelection()?.selectAllChildren(body);
  click();
  expect(activate).not.toHaveBeenCalled();
  window.getSelection()?.removeAllRanges();
  vi.advanceTimersToNextFrame();
  expect(activate).toHaveBeenCalledTimes(1);
});

it('stops handling clicks after disposal', () => {
  cleanup?.();
  cleanup = undefined;
  click();
  expect(activate).not.toHaveBeenCalled();
});

it('ignores cancelled touch gestures and context-menu releases', () => {
  for (const type of ['pointercancel', 'contextmenu']) {
    body.dispatchEvent(
      new PointerEvent('pointerdown', { bubbles: true, isPrimary: true, pointerType: 'touch' })
    );
    body.dispatchEvent(new Event(type));
    body.dispatchEvent(
      new PointerEvent('click', { bubbles: true, pointerType: 'touch', detail: 1 })
    );
  }
  vi.advanceTimersByTime(500);
  expect(activate).not.toHaveBeenCalled();
});

it('does not activate after a touch long press', () => {
  body.dispatchEvent(
    new PointerEvent('pointerdown', {
      bubbles: true,
      isPrimary: true,
      pointerType: 'touch'
    })
  );
  vi.advanceTimersByTime(600);
  body.dispatchEvent(
    new PointerEvent('click', {
      bubbles: true,
      pointerType: 'touch',
      detail: 1
    })
  );
  expect(activate).not.toHaveBeenCalled();
});
