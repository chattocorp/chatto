import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import FormDialog from './FormDialog.svelte';

describe('FormDialog', () => {
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

    q(container, 'form')?.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }));

    expect(onsubmit).toHaveBeenCalledOnce();
  });
});
