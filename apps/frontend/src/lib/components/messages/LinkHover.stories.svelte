<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, userEvent } from 'storybook/test';
  import LinkHover from './LinkHover.svelte';
  import UrlLink from './UrlLink.svelte';
  import MessageContent from '../MessageContent.svelte';
  const { Story } = defineMeta({ title: 'Messages/Link hover cards', component: LinkHover });
  const href = 'https://chat.example.test/chat/-/development';
</script>

<Story
  name="Local room"
  asChild
  play={async ({ canvas }) => {
    await userEvent.hover(canvas.getByRole('link'));
    await expect(await canvas.findByTestId('link-hover-card')).toBeVisible();
  }}
>
  <div class="prose p-4">
    <LinkHover
      {href}
      local={{
        name: '# Development',
        server: 'Chatto community',
        description: 'Discuss features, share progress, and help build Chatto.'
      }}
    >
      <UrlLink {href} />
    </LinkHover>
  </div>
</Story>

<Story
  name="Authored Markdown link"
  asChild
  play={async ({ canvas }) => {
    const link = canvas.getByRole('link', { name: 'the wonderful development channel' });
    await userEvent.hover(link);
    await expect(await canvas.findByTestId('link-hover-card')).toBeVisible();
    await expect(link).toHaveTextContent('the wonderful development channel');
  }}
>
  <div class="p-4">
    <MessageContent
      body="Please join [the **wonderful** development channel](https://example.test/development &quot;Development discussions&quot;)."
    />
  </div>
</Story>
