import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import FormDialog from './FormDialog.svelte';
import { page, userEvent } from 'vitest/browser';

describe('FormDialog', () => {
  it('keeps native validation and Enter submission in the mobile sheet', async () => {
    await page.viewport(390, 844);
    const onsubmit = vi.fn();
    const { container } = render(FormDialog, {
      visible: true,
      title: 'Create Room',
      children: testSnippet(
        '<div><label for="room-name">Room name</label><input id="room-name" required /></div>'
      ),
      onsubmit,
      onclose: vi.fn()
    });
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    expect(onsubmit).not.toHaveBeenCalled();
    await page.getByRole('textbox', { name: 'Room name' }).fill('General');
    await userEvent.keyboard('{Enter}');
    expect(onsubmit).toHaveBeenCalledOnce();
    expect(container.querySelector('dialog')!.open).toBe(true);
  });

  it('renders standard footer actions without a divider or cancel icon', async () => {
    const { container } = render(FormDialog, {
      props: {
        visible: true,
        title: 'Create Room',
        submitLabel: 'Create Room',
        children: testSnippet('<input name="name" />'),
        onsubmit: vi.fn(),
        onclose: vi.fn()
      }
    });

    const footer = q(container, 'footer');
    await expect.element(footer).toBeInTheDocument();
    expect(footer?.querySelector('[aria-hidden="true"]')).toBeNull();

    const cancel = Array.from(footer?.querySelectorAll('button') ?? []).find(
      (button) => button.textContent?.trim() === 'Cancel'
    );
    expect(cancel).toBeDefined();
    expect(cancel?.querySelector('.iconify')).toBeNull();
  });

  it('submits with Enter through the owned form', () => {
    const onsubmit = vi.fn();
    const { container } = render(FormDialog, {
      props: {
        visible: true,
        title: 'Create Room',
        submitLabel: 'Create Room',
        children: testSnippet('<input name="name" value="General" />'),
        onsubmit,
        onclose: vi.fn()
      }
    });

    q(container, 'form')?.dispatchEvent(
      new SubmitEvent('submit', { bubbles: true, cancelable: true })
    );

    expect(onsubmit).toHaveBeenCalledOnce();
  });
});
