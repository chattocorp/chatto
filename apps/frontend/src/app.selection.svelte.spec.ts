import './app.css';
import { cdp } from 'vitest/browser';
import { afterEach, expect, it } from 'vitest';

let fixture: HTMLElement | undefined;

afterEach(() => {
  fixture?.remove();
  fixture = undefined;
  window.getSelection()?.removeAllRanges();
});

/** Use real mouse input: DOM ranges can select text even when CSS blocks the user. */
async function dragAcross(element: HTMLElement) {
  const range = document.createRange();
  range.selectNodeContents(element);
  const bounds = range.getBoundingClientRect();
  const y = bounds.top + bounds.height / 2;
  const mouse = cdp();
  await mouse.send('Input.dispatchMouseEvent', {
    type: 'mousePressed',
    x: bounds.left,
    y,
    button: 'left',
    clickCount: 1
  });
  await mouse.send('Input.dispatchMouseEvent', {
    type: 'mouseMoved',
    x: bounds.right,
    y,
    button: 'left',
    buttons: 1
  });
  await mouse.send('Input.dispatchMouseEvent', {
    type: 'mouseReleased',
    x: bounds.right,
    y,
    button: 'left',
    clickCount: 1
  });
}

it('allows mouse selection of ordinary content without a selection override', async () => {
  fixture = document.createElement('section');
  const detail = document.createElement('p');
  detail.textContent = 'member@example.test';
  fixture.append(detail);
  document.body.append(fixture);

  await dragAcross(detail);

  expect(window.getSelection()?.toString()).toBe(detail.textContent);
});

it.each(['btn', 'menu-item', 'menu-entry'])(
  'does not select the label of a %s control during a drag',
  async (className) => {
    // Use a custom control so native button behavior cannot hide a missing CSS rule.
    fixture = document.createElement('div');
    fixture.setAttribute('role', 'button');
    fixture.className = className;
    fixture.textContent = 'Example action';
    document.body.append(fixture);

    await dragAcross(fixture);

    expect(window.getSelection()?.toString()).toBe('');
  }
);
