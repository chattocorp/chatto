import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import '../../../app.css';
import TipTapEditor from './TipTapEditor.svelte';
import MarkdownEditor from './MarkdownEditor.svelte';
import type { ComposerEditorApi } from './editorTypes';

const originalTheme = document.documentElement.dataset.theme;
const originalTransition = document.documentElement.style.transition;
afterEach(() => {
  if (originalTheme === undefined) delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = originalTheme;
  document.documentElement.style.transition = originalTransition;
});

it('keeps fenced code upright inside a source quote', async () => {
  let api: ComposerEditorApi | undefined;
  const { container } = render(MarkdownEditor, { onReady: (ready) => (api = ready) });
  await vi.waitFor(() => expect(api).toBeDefined());
  api!.setContent('> ```text\n> plain code\n> ```');
  await expect.poll(() => container.querySelector('.cm-code-fence-body')).not.toBeNull();
  const code = container.querySelector('.cm-code-fence-body .cm-format-code')!;
  expect(code).not.toBeNull();
  expect(getComputedStyle(code).fontStyle).toBe('normal');
});

it('uses the same preview typography in both composers across themes and edits', async () => {
  const apis: ComposerEditorApi[] = [];
  const visual = render(TipTapEditor, { onReady: (api) => apis.push(api) }).container;
  await vi.waitFor(() => expect(apis).toHaveLength(1));
  const source = render(MarkdownEditor, { onReady: (api) => apis.push(api) }).container;
  await vi.waitFor(() => expect(apis).toHaveLength(2));
  document.documentElement.style.transition = 'none';

  for (const delimiter of ['*', '_']) {
    const strongTag = delimiter === '_' ? 'u' : 'strong';
    const strongClass = delimiter === '_' ? 'cm-format-underline' : 'cm-format-strong';
    const markdown = `# Heading
${delimiter}${delimiter}Strong${delimiter}${delimiter} and ${delimiter}emphasis${delimiter}, \`code\`, [link](https://example.com).
> Quote with ${delimiter}${delimiter}strong${delimiter}${delimiter}, ${delimiter}emphasis${delimiter}, \`code\`, and [link](https://example.com).`;
    for (const api of apis) api.setContent(markdown);
    await expect
      .poll(() => source.querySelectorAll('.cm-format-emphasis').length)
      .toBeGreaterThan(0);

    for (const theme of ['light', 'dark', 'light']) {
      document.documentElement.dataset.theme = theme;
      const pairs = [
        ['h1', '.cm-format-heading:last-child'],
        [`p > ${strongTag}`, `.cm-line:nth-child(2) .${strongClass}`],
        ['p > em', '.cm-line:nth-child(2) .cm-format-emphasis'],
        ['p > code', '.cm-line:nth-child(2) .cm-format-code'],
        ['p > a', '.cm-line:nth-child(2) .cm-link-label'],
        ['blockquote', '.cm-line:nth-child(3) .cm-format-quote:nth-child(2)'],
        [`blockquote ${strongTag}`, `.cm-line:nth-child(3) .${strongClass} .cm-format-quote`],
        ['blockquote em', '.cm-format-quote.cm-format-emphasis'],
        ['blockquote code', '.cm-format-quote.cm-format-code']
      ];
      for (const [visualSelector, sourceSelector] of pairs) {
        const left = visual.querySelector(visualSelector!)!;
        const right = source.querySelector(`${sourceSelector}:not(.cm-format-marker)`)!;
        expect(left, visualSelector).not.toBeNull();
        expect(right, sourceSelector).not.toBeNull();
        const expected = getComputedStyle(left);
        const actual = getComputedStyle(right);
        for (const property of [
          'color',
          'font-family',
          'font-size',
          'font-weight',
          'font-style',
          'text-decoration-line',
          'text-underline-offset',
          'background-color'
        ]) {
          // Underline belongs to the decoration wrapper and propagates to its
          // descendants; computed styles on a quote's inner span omit it.
          const sourceStyle =
            property === 'text-decoration-line' && right.closest('.cm-format-underline')
              ? getComputedStyle(right.closest('.cm-format-underline')!)
              : actual;
          expect(
            sourceStyle.getPropertyValue(property),
            `${theme}: ${sourceSelector} ${property}`
          ).toBe(expected.getPropertyValue(property));
        }
      }
      const quoteLink = getComputedStyle(visual.querySelector('blockquote a')!);
      const sourceLink = source.querySelector('.cm-line:nth-child(3) .cm-link-label')!;
      const sourceLinkText = getComputedStyle(sourceLink.querySelector('.cm-format-quote')!);
      expect(sourceLinkText.color).toBe(quoteLink.color);
      expect(sourceLinkText.fontStyle).toBe(quoteLink.fontStyle);
      expect(getComputedStyle(sourceLink).textDecorationLine).toBe(quoteLink.textDecorationLine);
    }
    expect(apis[1]!.getText()).toBe(markdown);
  }
});
