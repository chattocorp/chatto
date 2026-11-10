<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, within, userEvent, waitFor } from 'storybook/test';
  import FormattingExample from './FormattingExample.svelte';

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
