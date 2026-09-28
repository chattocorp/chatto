import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import '../../app.css';
import InputModalityTracker from './InputModalityTracker.svelte';

const root = document.documentElement;

describe('InputModalityTracker', () => {
  let fixture: HTMLDivElement;

  beforeEach(() => {
    fixture = document.createElement('div');
    fixture.innerHTML = `
      <button type="button" data-testid="mini" class="mini-icon-action">Mini</button>
      <button type="button" data-testid="plain">Plain</button>
      <button type="button" data-testid="danger" class="icon-action icon-action-danger">Danger</button>
      <input type="text" data-testid="text" aria-label="Text" />
    `;
    document.body.append(fixture);
  });

  afterEach(() => {
    fixture.remove();
    root.removeAttribute('data-input-modality');
  });

  const button = (testId: string) =>
    fixture.querySelector<HTMLButtonElement>(`[data-testid="${testId}"]`)!;

  const pointerDown = () =>
    document.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));

  it('sets pointer mode on pointer input and clears it on a key press', async () => {
    render(InputModalityTracker);
    expect(root.hasAttribute('data-input-modality')).toBe(false);

    await userEvent.click(button('plain'));
    expect(root.getAttribute('data-input-modality')).toBe('pointer');

    await userEvent.keyboard('{Control>}c{/Control}');
    expect(root.getAttribute('data-input-modality')).toBe('pointer');

    await userEvent.keyboard('{Tab}');
    expect(root.hasAttribute('data-input-modality')).toBe(false);
  });

  it('removes the attribute when it unmounts', async () => {
    const { unmount } = render(InputModalityTracker);
    await userEvent.click(button('plain'));
    expect(root.getAttribute('data-input-modality')).toBe('pointer');

    unmount();
    expect(root.hasAttribute('data-input-modality')).toBe(false);
  });

  it('hides focus styles after pointer input even when :focus-visible matches', async () => {
    render(InputModalityTracker);

    for (const testId of ['mini', 'plain', 'danger']) {
      const target = button(testId);
      target.focus();
      // Keyboard focus: the browser and the tracker agree.
      await userEvent.keyboard('{Shift}');
      expect(target.matches(':focus-visible')).toBe(true);
      const keyboardStyle = getComputedStyle(target);
      expect(keyboardStyle.outlineStyle, testId).not.toBe('none');
      const keyboardColor = keyboardStyle.color;

      // A synthetic pointerdown does not change the browser's heuristic, so
      // :focus-visible still matches, like script focus after a tap on iOS.
      pointerDown();
      expect(target.matches(':focus-visible')).toBe(true);
      const pointerStyle = getComputedStyle(target);
      expect(pointerStyle.outlineStyle, testId).toBe('none');
      if (testId === 'danger') expect(pointerStyle.color).not.toBe(keyboardColor);
    }
  });

  it('keeps the default focus outline on text fields after pointer input', async () => {
    render(InputModalityTracker);
    const field = fixture.querySelector<HTMLInputElement>('[data-testid="text"]')!;
    field.focus();
    pointerDown();

    expect(root.getAttribute('data-input-modality')).toBe('pointer');
    expect(field.matches(':focus-visible')).toBe(true);
    expect(getComputedStyle(field).outlineStyle).not.toBe('none');
  });
});
