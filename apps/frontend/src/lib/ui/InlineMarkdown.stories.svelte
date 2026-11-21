<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect } from 'storybook/test';
  import InlineMarkdown from './InlineMarkdown.svelte';

  const { Story } = defineMeta({
    title: 'UI/Inline Markdown',
    component: InlineMarkdown,
    tags: ['autodocs']
  });

  const description =
    "A small Chatto server running the very latest code from Chatto's `main` branch.\n**Expect bugs.**\n\nSee [the changelog](https://example.com) for what's new.";
</script>

<Story
  name="Server description"
  asChild
  play={async ({ canvas, canvasElement }) => {
    await expect(await canvas.findByRole('link', { name: 'the changelog' })).toHaveAttribute(
      'href',
      'https://example.com'
    );
    await expect(canvasElement.querySelector('code')).toHaveTextContent('main');
    await expect(canvasElement.querySelector('br')).not.toBeNull();
    await expect(canvasElement.querySelectorAll('p')).toHaveLength(2);
  }}
>
  <div class="prose prose-center w-96 max-w-full text-muted" dir="auto">
    <InlineMarkdown text={description} multiline />
  </div>
</Story>

<Story name="Single line" asChild>
  <div class="prose w-96 max-w-full text-muted" dir="auto">
    <InlineMarkdown text={description} />
  </div>
</Story>
