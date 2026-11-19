<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, within, userEvent, waitFor } from 'storybook/test';
  import FormattingExample from './FormattingExample.svelte';
  import HelpDocument from './HelpDocument.svelte';
  import MarkdownSample from './MarkdownSample.svelte';

  const { Story } = defineMeta({ title: 'Help/Interactive guides', component: FormattingExample });
</script>

<Story
  name="Formatting"
  asChild
  play={async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const preview = await canvas.findByRole('region', { name: 'Preview' });
    await waitFor(() => expect(preview).toHaveTextContent('Hello everyone!'));
    // Inline emphasis must not become a separate flex item, as it did in the first prototype.
    const emphasis = preview.querySelector('strong')!;
    const prefix = document.createRange();
    prefix.selectNodeContents(emphasis.parentElement!.firstChild!);
    expect(
      Math.abs(prefix.getBoundingClientRect().top - emphasis.getBoundingClientRect().top)
    ).toBeLessThan(3);
    await userEvent.click(canvas.getByRole('button', { name: 'Bold **Bold** Bold' }));
    await waitFor(() => expect(preview.querySelector('strong')).toHaveTextContent('Bold'));
    await userEvent.click(canvas.getByRole('radio', { name: 'Visual' }));
    await waitFor(() =>
      expect(canvas.getByRole('textbox', { name: 'Message text' })).toHaveTextContent('Bold')
    );
  }}
>
  <div class="flex max-w-5xl flex-col gap-10 p-6"><FormattingExample /></div>
</Story>

<Story
  name="Code overflow cues"
  asChild
  play={async ({ canvasElement }) => {
    for (const frame of canvasElement.querySelectorAll('pre')) {
      const viewport = frame.querySelector('code')!;
      const left = frame.querySelector('.markdown-code-fade-left');
      const right = frame.querySelector('.markdown-code-fade-right');
      await waitFor(() => {
        expect(viewport.scrollWidth).toBeGreaterThan(viewport.clientWidth);
        expect(left).toHaveClass('opacity-0');
        expect(right).not.toHaveClass('opacity-0');
      });
      viewport.scrollLeft = viewport.scrollWidth;
      await waitFor(() => {
        expect(left).not.toHaveClass('opacity-0');
        expect(right).toHaveClass('opacity-0');
      });
    }
  }}
>
  <div class="w-80 p-6">
    <HelpDocument locale="en-GB">
      <pre><code>A long code example with enough text to overflow the narrow article column.</code
        ></pre>
      <MarkdownSample
        source="A long Markdown source example with enough text to overflow the narrow source column."
      />
    </HelpDocument>
  </div>
</Story>

<Story
  name="Right-to-left article"
  asChild
  play={async ({ canvasElement }) => {
    const document = canvasElement.querySelector('[lang="ar"]')!;
    expect(getComputedStyle(document).direction).toBe('rtl');
    for (const code of document.querySelectorAll('code')) {
      expect(getComputedStyle(code).direction).toBe('ltr');
      expect(getComputedStyle(code).unicodeBidi).toBe('isolate');
    }
    expect(getComputedStyle(document.querySelector('pre')!).direction).toBe('ltr');
  }}
>
  <div class="max-w-prose p-6">
    <HelpDocument locale="ar">
      <h2>اربط أداتك</h2>
      <p>امنح البوت الإذن <code>message.post</code> في الغرفة المطلوبة.</p>
      <pre><code>Authorization: Bearer YOUR_API_KEY</code></pre>
    </HelpDocument>
  </div>
</Story>
