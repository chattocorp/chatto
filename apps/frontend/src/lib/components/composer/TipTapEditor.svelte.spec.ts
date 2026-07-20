import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it } from 'vitest';
import '../../../app.css';
import TipTapEditor from './TipTapEditor.svelte';

describe('TipTapEditor accessibility', () => {
  it('keeps its accessible name synchronized with the placeholder', async () => {
    const rendered = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    await expect.element(page.getByRole('textbox', { name: 'Write a message' })).toBeVisible();

    await rendered.rerender({ placeholder: 'Edit your message' });

    await expect.element(page.getByRole('textbox', { name: 'Edit your message' })).toBeVisible();
  });
});

describe('TipTapEditor wrapping', () => {
  it('uses stable wrapping instead of global prose wrapping', async () => {
    const { container } = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    await expect.element(page.getByRole('textbox', { name: 'Write a message' })).toBeVisible();

    const paragraph = container.querySelector('.ProseMirror p');
    expect(paragraph).toBeInstanceOf(HTMLParagraphElement);
    expect(getComputedStyle(paragraph!).textWrap).toBe('wrap');
  });
});
