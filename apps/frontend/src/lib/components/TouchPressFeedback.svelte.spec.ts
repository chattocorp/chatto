import { afterEach, beforeEach, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import '../../app.css';
import TouchPressFeedback from './TouchPressFeedback.svelte';
import MenuItemTestHarness from '../ui/MenuItemTestHarness.svelte';
import SegmentedControl from '../ui/SegmentedControl.svelte';

let fixture: HTMLDivElement;
let previousAnimations: string | undefined;
let previousMotion: string | undefined;

beforeEach(() => {
  previousAnimations = document.documentElement.dataset.animations;
  previousMotion = document.documentElement.dataset.motion;
  document.documentElement.dataset.animations = 'enabled';
  document.documentElement.dataset.motion = 'full';
  fixture = document.createElement('div');
  fixture.innerHTML = `
    <div role="button" id="outer"><button id="action"><span id="icon">Action</span></button></div>
    <button disabled id="disabled">Disabled</button>
    <a href="#" aria-disabled="true" id="disabled-link">Disabled link</a>
    <label id="choice"><input type="checkbox">Choice</label>
    <label id="disabled-choice"><input type="checkbox" disabled>Disabled choice</label>
    <div contenteditable="true"><a href="#" id="editable">Editable link</a></div>
    <span data-press-feedback id="custom">Clickable mention</span>
    <span id="text">Plain message text</span>
    <div data-press-feedback="none" id="excluded"><button id="drag">Drag handle</button></div>
    <span role="menuitemcheckbox" id="menu-checkbox" tabindex="0">Menu checkbox</span>
    <span role="menuitemradio" id="menu-radio" tabindex="0">Menu radio</span>
    <input id="native-checkbox" type="checkbox">
    <input id="text-input" type="text">
    <article data-press-surface id="card">
      <button data-press-feedback="surface" id="card-overlay">Open card</button>
      <div data-press-feedback="surface" id="card-description">Description</div>
      <button id="card-nested">Separate action</button>
    </article>
    <table><tbody data-table-row-links><tr id="linked-row">
      <td><a class="data-table-row-link" href="#" id="row-link">Details</a></td>
      <td id="passive-cell">Description</td>
      <td><button id="row-action">Separate action</button><input id="row-input" type="text"></td>
    </tr><tr id="plain-row"><td>Not navigable</td></tr></tbody></table>
  `;
  document.body.append(fixture);
});

afterEach(() => {
  fixture.remove();
  if (previousAnimations === undefined) delete document.documentElement.dataset.animations;
  else document.documentElement.dataset.animations = previousAnimations;
  if (previousMotion === undefined) delete document.documentElement.dataset.motion;
  else document.documentElement.dataset.motion = previousMotion;
});

function pointer(type: string, target: EventTarget, options: PointerEventInit = {}) {
  target.dispatchEvent(
    new PointerEvent(type, {
      bubbles: true,
      pointerType: 'touch',
      pointerId: 1,
      isPrimary: true,
      clientX: 20,
      clientY: 20,
      ...options
    })
  );
}

const element = (id: string) => fixture.querySelector<HTMLElement>(`#${id}`)!;

it('shows immediate feedback only on the innermost action and clears it on release', () => {
  render(TouchPressFeedback);
  pointer('pointerdown', element('icon'));
  expect(getComputedStyle(element('action')).opacity).toBe('0.65');
  expect(getComputedStyle(element('outer')).opacity).toBe('1');
  pointer('pointerup', document);
  expect(element('action').hasAttribute('data-touch-pressed')).toBe(false);
});

it.each(['touch', 'mouse', 'pen'])('shows feedback for primary %s presses', (pointerType) => {
  render(TouchPressFeedback);
  pointer('pointerdown', element('action'), { pointerType });
  expect(element('action').hasAttribute('data-touch-pressed')).toBe(true);
  expect(element('action').querySelector('.touch-ripple')).not.toBeNull();
});

it('excludes secondary buttons, disabled controls, editable content, and opted-out surfaces', () => {
  render(TouchPressFeedback);
  for (const button of [1, 2]) {
    pointer('pointerdown', element('action'), { pointerType: 'mouse', button });
    expect(fixture.querySelector('[data-touch-pressed]')).toBeNull();
    expect(fixture.querySelector('.touch-ripple')).toBeNull();
  }
  for (const id of [
    'disabled',
    'disabled-link',
    'disabled-choice',
    'editable',
    'drag',
    'excluded',
    'text',
    'text-input'
  ]) {
    pointer('pointerdown', element(id));
    expect(fixture.querySelector('[data-touch-pressed]')).toBeNull();
    expect(fixture.querySelector('.touch-ripple')).toBeNull();
  }
  pointer('pointerdown', element('choice').querySelector('input')!);
  expect(getComputedStyle(element('choice')).opacity).toBe('0.65');
});

it('covers custom inline actions, ARIA menu choices, and standalone native controls', () => {
  render(TouchPressFeedback);
  for (const id of ['custom', 'menu-checkbox', 'menu-radio']) {
    const action = element(id);
    const rect = action.getBoundingClientRect();
    pointer('pointerdown', action, { clientX: rect.left + 2, clientY: rect.top + 2 });
    expect(getComputedStyle(action).opacity).toBe('0.65');
    const circle = action.querySelector<HTMLElement>('.touch-ripple-circle')!;
    expect(parseFloat(circle.style.width)).toBeGreaterThan(rect.width);
  }
  pointer('pointerdown', element('native-checkbox'));
  expect(getComputedStyle(element('native-checkbox')).opacity).toBe('0.65');
  expect(element('native-checkbox').childElementCount).toBe(0);
});

it('moves wrapping inline text without changing its line breaks and restores it on release', () => {
  render(TouchPressFeedback);
  const wrapper = document.createElement('div');
  wrapper.style.width = '100px';
  const link = document.createElement('a');
  link.href = '#';
  link.textContent = 'An inline link that wraps over several lines';
  wrapper.append(link);
  fixture.append(wrapper);
  const before = [...link.getClientRects()];
  expect(before.length).toBeGreaterThan(1);
  pointer('pointerdown', link, { clientX: before[0].x + 5, clientY: before[0].y + 5 });
  const during = [...link.getClientRects()];
  expect(during).toHaveLength(before.length);
  during.forEach((rect, i) => {
    expect(rect.x).toBeCloseTo(before[i].x + 1);
    expect(rect.y).toBeCloseTo(before[i].y + 1);
    expect(rect.width).toBeCloseTo(before[i].width);
  });
  pointer('pointerup', document);
  expect(link.getBoundingClientRect().x).toBeCloseTo(before[0].x);
  expect(link.hasAttribute('data-press-inline')).toBe(false);
});

it('uses the visible surface for proxy actions and keeps nested controls independent', () => {
  render(TouchPressFeedback);
  for (const id of ['card-overlay', 'card-description']) {
    pointer('pointerdown', element(id));
    expect(element('card').hasAttribute('data-touch-pressed')).toBe(true);
    expect(element(id).hasAttribute('data-touch-pressed')).toBe(false);
  }
  pointer('pointerdown', element('card-nested'));
  expect(element('card-nested').hasAttribute('data-touch-pressed')).toBe(true);
  expect(element('card').hasAttribute('data-touch-pressed')).toBe(false);
});

it('covers linked rows with cell-local ripple layers and leaves other controls alone', () => {
  render(TouchPressFeedback);
  for (const id of ['row-link', 'passive-cell']) {
    pointer('pointerdown', element(id));
    const row = element('linked-row');
    expect(row.hasAttribute('data-touch-pressed')).toBe(true);
    expect(row.querySelectorAll(':scope > td > .touch-ripple')).toHaveLength(3);
    expect(row.querySelector(':scope > .touch-ripple')).toBeNull();
    pointer('pointermove', document, { clientX: 40 });
    expect(row.querySelector('.touch-ripple')).toBeNull();
  }
  pointer('pointerdown', element('row-action'));
  expect(element('row-action').hasAttribute('data-touch-pressed')).toBe(true);
  expect(element('linked-row').hasAttribute('data-touch-pressed')).toBe(false);
  for (const id of ['plain-row', 'row-input']) {
    pointer('pointerdown', element(id));
    expect(fixture.querySelector('[data-touch-pressed]')).toBeNull();
  }
});

it('clips the segmented-control ripple to its visible shape above the state fill', () => {
  render(TouchPressFeedback);
  const { container } = render(SegmentedControl, {
    label: 'View',
    options: [{ value: 'all', label: 'All' }],
    value: 'all',
    onchange: () => {}
  });
  const input = container.querySelector('input')!;
  const label = input.closest('label')!;
  pointer('pointerdown', input);
  const ripple = label.querySelector<HTMLElement>('.touch-ripple')!;
  const face = label.querySelector<HTMLElement>('span:not(.touch-ripple)')!;
  expect(Number(getComputedStyle(ripple).zIndex)).toBeGreaterThan(
    Number(getComputedStyle(face).zIndex)
  );
  expect(getComputedStyle(ripple).borderRadius).not.toBe('0px');
});

it.each(['sheet', 'floating'] as const)(
  'shows feedback inside a native %s menu without blocking its action',
  async (presentation) => {
    render(TouchPressFeedback);
    render(MenuItemTestHarness, { presentation });
    const item = document.querySelector<HTMLElement>('[data-testid="icon-button"]')!;
    await expect.poll(() => item.getBoundingClientRect().width).toBeGreaterThan(0);
    pointer('pointerdown', item.querySelector('span')!);
    expect(getComputedStyle(item).opacity).toBe('0.65');
    expect(item.querySelector('.touch-ripple')).not.toBeNull();
    pointer('pointerup', item);
    item.click();
    await expect
      .poll(() => document.querySelector('[data-testid="click-count"]')?.textContent)
      .toBe('1');
  }
);

it('clears on movement, cancellation, additional touches, blur, and disposal', () => {
  const { unmount } = render(TouchPressFeedback);
  const action = element('action');
  pointer('pointerdown', action);
  pointer('pointermove', document, { clientX: 24 });
  expect(action.hasAttribute('data-touch-pressed')).toBe(true);
  pointer('pointermove', document, { clientX: 30 });
  expect(action.hasAttribute('data-touch-pressed')).toBe(false);
  expect(action.querySelector('.touch-ripple')).toBeNull();
  pointer('pointermove', document);
  expect(action.hasAttribute('data-touch-pressed')).toBe(false);

  pointer('pointerdown', action);
  pointer('pointercancel', document);
  expect(action.hasAttribute('data-touch-pressed')).toBe(false);
  expect(action.querySelector('.touch-ripple')).toBeNull();
  pointer('pointerdown', action);
  pointer('pointerdown', action, { pointerId: 2, isPrimary: false });
  expect(action.hasAttribute('data-touch-pressed')).toBe(false);
  pointer('pointerdown', action);
  window.dispatchEvent(new Event('blur'));
  expect(action.hasAttribute('data-touch-pressed')).toBe(false);
  pointer('pointerdown', action);
  unmount();
  expect(action.hasAttribute('data-touch-pressed')).toBe(false);
  expect(action.querySelector('.touch-ripple')).toBeNull();
  expect(action.hasAttribute('data-touch-ripple-host')).toBe(false);
});

it('starts at the touch point, finishes after release, and removes the decorative layer', async () => {
  render(TouchPressFeedback);
  const action = element('action');
  action.style.width = '100px';
  action.style.height = '40px';
  const rect = action.getBoundingClientRect();
  pointer('pointerdown', action, { clientX: rect.left + 8, clientY: rect.top + 6 });
  const layer = action.querySelector<HTMLElement>('.touch-ripple')!;
  const circle = layer.firstElementChild as HTMLElement;
  const animation = circle.getAnimations()[0];
  expect(layer.getAttribute('aria-hidden')).toBe('true');
  expect(getComputedStyle(layer).pointerEvents).toBe('none');
  expect(parseFloat(circle.style.left) + parseFloat(circle.style.width) / 2).toBeCloseTo(
    8 - action.clientLeft
  );
  expect(parseFloat(circle.style.top) + parseFloat(circle.style.height) / 2).toBeCloseTo(
    6 - action.clientTop
  );
  pointer('pointerup', document);
  expect(layer.isConnected).toBe(true);
  animation.finish();
  await expect.poll(() => layer.isConnected).toBe(false);
  expect(action.hasAttribute('data-touch-ripple-host')).toBe(false);
});

it('restarts for repeated taps and skips the ripple with reduced motion', () => {
  render(TouchPressFeedback);
  const action = element('action');
  pointer('pointerdown', action);
  const first = action.querySelector('.touch-ripple')!;
  pointer('pointerup', document);
  pointer('pointerdown', action);
  expect(first.isConnected).toBe(false);
  expect(action.querySelectorAll('.touch-ripple')).toHaveLength(1);
  document.documentElement.dataset.animations = 'disabled';
  document.documentElement.dataset.motion = 'reduced';
  pointer('pointerdown', action);
  expect(action.querySelector('.touch-ripple')).toBeNull();
  expect(getComputedStyle(action).opacity).toBe('0.65');
});
