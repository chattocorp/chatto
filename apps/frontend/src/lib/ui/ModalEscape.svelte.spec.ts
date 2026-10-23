import '../../app.css';
import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import ModalEscapeHarness from './ModalEscapeHarness.svelte';

describe('modal Escape ownership', () => {
  it('closes the modal on the first Escape when the combobox has no visible dropdown', async () => {
    const { container } = render(ModalEscapeHarness);
    await page.getByRole('combobox').click();
    await userEvent.keyboard('{Escape}');
    await expect.element(container.querySelector('dialog')).not.toBeVisible();
    await expect.element(page.getByText('Background pane')).toBeVisible();
  });

  it('dismisses a visible dropdown before the modal and leaves the background pane open', async () => {
    const { container } = render(ModalEscapeHarness, { props: { items: ['Alice'] } });
    await page.getByRole('combobox').click();
    await userEvent.keyboard('{ArrowDown}');
    await expect.element(page.getByRole('listbox')).toBeVisible();
    await userEvent.keyboard('{Escape}');
    await expect
      .element(container.querySelector<HTMLElement>('[role="listbox"]'))
      .not.toBeVisible();
    await expect.element(page.getByRole('dialog')).toBeVisible();
    await expect.element(page.getByText('Background pane')).toBeVisible();
    await userEvent.keyboard('{Escape}');
    await expect.element(container.querySelector('dialog')).not.toBeVisible();
    await expect.element(page.getByText('Background pane')).toBeVisible();
    await userEvent.keyboard('{Escape}');
    await expect.element(page.getByText('Background pane')).not.toBeInTheDocument();
  });
});
