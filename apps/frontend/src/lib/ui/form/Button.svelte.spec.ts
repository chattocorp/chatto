import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import Button from './Button.svelte';

describe('Button', () => {
  it('keeps translated labels on one line without shrinking', async () => {
    const { container } = render(Button, {
      props: {
        children: testSnippet('<span>Schlüssel widerrufen</span>')
      }
    });

    const button = container.querySelector('button');
    await expect.element(button).toHaveClass('whitespace-nowrap');
    await expect.element(button).toHaveClass('shrink-0');
  });
});
