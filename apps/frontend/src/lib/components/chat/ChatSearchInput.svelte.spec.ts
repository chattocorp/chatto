import { describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import ChatSearchInputTestHarness from './ChatSearchInputTestHarness.svelte';

describe('ChatSearchInput', () => {
  it('binds its value, reports input, and submits with Enter', async () => {
    const rendered = render(ChatSearchInputTestHarness);
    const input = rendered.getByRole('searchbox', { name: 'Search messages' });
    const surface = rendered.container.querySelector('form')!;

    expect(surface).toHaveClass('h-12', 'chat-input-surface');
    expect(surface.className).not.toContain('focus-within:ring');

    await userEvent.fill(input, 'roadmap');
    await expect.element(rendered.getByTestId('search-value')).toHaveTextContent('roadmap');
    await expect.element(rendered.getByTestId('input-count')).toHaveTextContent('1');
    expect(rendered.getByRole('button', { name: 'Clear search' }).element()).toHaveClass(
      'h-8',
      'w-8'
    );
    expect(surface).toHaveClass('h-12', 'chat-input-surface');

    await userEvent.type(input, '{Enter}');
    await expect.element(rendered.getByTestId('submit-count')).toHaveTextContent('1');
  });

  it('clears the value and restores focus', async () => {
    const rendered = render(ChatSearchInputTestHarness, {
      props: { value: 'roadmap' }
    });
    const input = rendered.getByRole('searchbox', { name: 'Search messages' });

    await userEvent.click(rendered.getByRole('button', { name: 'Clear search' }));

    await expect.element(input).toHaveValue('');
    await expect.element(input).toHaveFocus();
    await expect.element(rendered.getByTestId('clear-count')).toHaveTextContent('1');
  });

  it('focuses on mount and exposes its disabled state', async () => {
    const focused = render(ChatSearchInputTestHarness, { props: { focusOnMount: true } });
    await expect
      .element(focused.getByRole('searchbox', { name: 'Search messages' }))
      .toHaveFocus();
    focused.unmount();

    const disabled = render(ChatSearchInputTestHarness, { props: { disabled: true } });
    await expect
      .element(disabled.getByRole('searchbox', { name: 'Search messages' }))
      .toBeDisabled();
  });
});
