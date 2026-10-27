import '../../../app.css';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import Button from './Button.svelte';

describe('Button', () => {
  it('renders small primary buttons at 32px instead of the default 40px height', async () => {
    const view = render(Button, {
      props: { size: 'sm', children: testSnippet('<span>Dismiss</span>') }
    });
    const button = view.container.querySelector('button')!;
    expect(button.getBoundingClientRect().height).toBe(32);
    await view.rerender({ size: 'xs' });
    expect(button.getBoundingClientRect().height).toBe(32);
    await view.rerender({ size: 'compact' });
    expect(button.getBoundingClientRect().height).toBe(24);
    await view.rerender({ size: 'md' });
    expect(button.getBoundingClientRect().height).toBe(40);
  });

  it('keeps translated content in one centered flex row', async () => {
    const { container } = render(Button, {
      props: {
        children: testSnippet('<span>Schlüssel widerrufen</span>')
      }
    });

    const button = container.querySelector('button');
    await expect.element(button).toHaveClass('whitespace-nowrap');
    await expect.element(button).toHaveClass('shrink-0');
    const content = button!.querySelector<HTMLElement>('.button-content');
    await expect.element(content).toHaveTextContent('Schlüssel widerrufen');
    await expect.element(content).toHaveClass('button-content');
    await expect.element(content).toHaveClass('inline-flex');
    await expect.element(content).toHaveClass('items-center');
    await expect.element(content).toHaveClass('gap-2');
    await expect.element(content).toHaveClass('[&>.iconify]:shrink-0');
  });

  it('renders a square icon-only button named by its label', async () => {
    const screen = render(Button, {
      props: {
        size: 'icon',
        label: 'Save status',
        children: testSnippet('<span class="iconify icon-[uil--check]" aria-hidden="true"></span>')
      }
    });
    const button = screen.getByRole('button', { name: 'Save status' });

    await expect.element(button).toHaveClass('btn-icon');
    const style = getComputedStyle(button.element());
    expect(style.width).toBe('40px');
    expect(style.height).toBe('40px');
    expect(style.paddingInlineStart).toBe('0px');
  });
});
