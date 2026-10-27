import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import '../../app.css';
import MessageContent from './MessageContent.svelte';
import TipTapEditor from './composer/TipTapEditor.svelte';
import type { ComposerEditorApi } from './composer/editorTypes';

vi.mock('$app/navigation', () => ({ goto: vi.fn(), pushState: vi.fn() }));
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults
}));

let originalTheme: string | undefined;
let originalTransition: string;
beforeEach(() => {
  originalTheme = document.documentElement.dataset.theme;
  originalTransition = document.documentElement.style.transition;
  // Read settled colours; palette transitions have their own tests.
  document.documentElement.style.transition = 'none';
});
afterEach(() => {
  if (originalTheme === undefined) delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = originalTheme;
  document.documentElement.style.transition = originalTransition;
});

describe.each(['*', '_'])('Blockquote emphasis with %s', (delimiter) => {
  it.each(['message', 'composer'])(
    'keeps emphasis distinct in the %s across themes',
    async (surface) => {
      const source = `Text ${delimiter}marked with stars${delimiter} is emphasised.

> Quoted text ${delimiter}marked with stars${delimiter} is emphasised.
>
> Second paragraph with ${delimiter}${delimiter}strong${delimiter}${delimiter}, \`code\`, and [link](https://example.com).
>
> > Nested ${delimiter}emphasis${delimiter} and ${delimiter}${delimiter}strong${delimiter}${delimiter}.
>
> - Bullet ${delimiter}emphasis${delimiter}
> - Bullet ${delimiter}${delimiter}strong${delimiter}${delimiter}
>
> 1. Ordered ${delimiter}emphasis${delimiter}
> 2. Ordered ${delimiter}${delimiter}strong${delimiter}${delimiter}`;
      let container: HTMLElement;
      if (surface === 'message') {
        container = render(MessageContent, { body: source }).container;
      } else {
        const apis: ComposerEditorApi[] = [];
        container = render(TipTapEditor, {
          placeholder: 'Write a quote',
          onReady: (api: ComposerEditorApi) => apis.push(api)
        }).container;
        await vi.waitFor(() => expect(apis).toHaveLength(1));
        apis[0]!.setContent(source);
      }
      await expect.poll(() => container.querySelectorAll('blockquote em').length).toBe(4);

      // Switch the theme on the mounted content, as App Preferences does.
      const quoteColours: string[] = [];
      for (const theme of ['light', 'dark', 'light']) {
        document.documentElement.dataset.theme = theme;
        const outside = container.querySelector('p')!;
        const outsideStyle = getComputedStyle(outside);
        expect(outsideStyle.fontStyle).toBe('normal');
        expect(getComputedStyle(outside.querySelector('em')!).fontStyle).toBe('italic');

        const quotes = container.querySelectorAll('blockquote');
        expect(quotes).toHaveLength(2);
        for (const quote of quotes) {
          const style = getComputedStyle(quote);
          expect(style.fontStyle).toBe('italic');
          expect(style.color).not.toBe(outsideStyle.color);
          expect(style.borderInlineStartStyle).toBe('solid');
          expect(parseFloat(style.borderInlineStartWidth)).toBeGreaterThan(0);
          expect(style.borderInlineStartColor).not.toBe('rgba(0, 0, 0, 0)');
          expect(parseFloat(style.paddingInlineStart)).toBeGreaterThan(0);
        }
        quoteColours.push(getComputedStyle(quotes[0]!).color);

        expect(quotes[0]!.querySelectorAll(':scope > p')).toHaveLength(2);
        expect(quotes[0]!.querySelectorAll('ul > li')).toHaveLength(2);
        expect(quotes[0]!.querySelectorAll('ol > li')).toHaveLength(2);
        for (const paragraph of container.querySelectorAll('blockquote p, blockquote li')) {
          expect(getComputedStyle(paragraph).fontStyle).toBe('italic');
        }
        for (const emphasis of container.querySelectorAll('blockquote em')) {
          expect(getComputedStyle(emphasis).fontStyle).toBe('normal');
        }
        const emphasisWeight = getComputedStyle(quotes[0]!.querySelector('em')!).fontWeight;
        const strongElements = container.querySelectorAll(
          delimiter === '_' ? 'blockquote u' : 'blockquote strong'
        );
        expect(strongElements).toHaveLength(4);
        for (const strong of strongElements) {
          const style = getComputedStyle(strong);
          expect(style.fontStyle).toBe('italic');
          if (delimiter === '_') {
            expect(style.textDecorationLine).toContain('underline');
            expect(style.fontWeight).toBe(emphasisWeight);
          } else {
            expect(Number(style.fontWeight)).toBeGreaterThan(Number(emphasisWeight));
          }
        }
        const code = quotes[0]!.querySelector('code')!;
        expect(getComputedStyle(code).fontStyle).toBe('normal');
        expect(getComputedStyle(code).fontFamily).toContain('Mono');
        const link = quotes[0]!.querySelector('a')!;
        expect(link.getAttribute('href')).toBe('https://example.com');
        expect(getComputedStyle(link).textDecorationLine).toContain('underline');
      }
      expect(quoteColours[0]).not.toBe(quoteColours[1]);
      expect(quoteColours[2]).toBe(quoteColours[0]);
    }
  );
});
