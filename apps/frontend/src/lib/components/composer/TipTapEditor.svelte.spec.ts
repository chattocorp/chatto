import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it } from 'vitest';
import TipTapEditor from './TipTapEditor.svelte';

describe('TipTapEditor accessibility', () => {
  it('keeps its accessible name synchronized with the placeholder', async () => {
    const rendered = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    await expect.element(page.getByRole('textbox', { name: 'Write a message' })).toBeVisible();

    await rendered.rerender({ placeholder: 'Edit your message' });

    await expect.element(page.getByRole('textbox', { name: 'Edit your message' })).toBeVisible();
  });
});
