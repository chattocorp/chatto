import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import Button from './Button.svelte';

describe('Button', () => {
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
});
