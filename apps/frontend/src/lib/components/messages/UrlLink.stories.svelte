<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import UrlLink from './UrlLink.svelte';
  import TipTapEditor from '../composer/TipTapEditor.svelte';

  const { Story } = defineMeta({ title: 'Messages/URL chips', component: UrlLink });
  const longLink = 'http://chatto.ws55110.localhost:55110/chat/-/user/UT9SQd0NQaXj3pc';
</script>

{#snippet examples()}
  <p>Please join <UrlLink href={longLink} /> and visit <UrlLink href="https://example.com" />.</p>
  <p>
    Documentation: <UrlLink
      href="https://docs.example.com/a/long/path/to/the/getting-started/guide?language=en#installation"
    />.
  </p>
  <p>
    Long hostname: <UrlLink
      href="https://a-very-long-subdomain.another-long-subdomain.example.com/path"
    />.
  </p>
  <p>
    Page title: <UrlLink href="https://example.com/guide" title="Getting started with Chatto" />.
  </p>
  <p>
    Long title: <UrlLink
      href="https://example.com/guide"
      title="A comprehensive guide to setting up your server and welcoming your community"
    />.
  </p>
{/snippet}

<Story name="Automatic web links" asChild>
  <div class="prose max-w-xl bg-background p-4">{@render examples()}</div>
</Story>
<Story name="Narrow message" asChild>
  <div class="prose w-64 bg-background p-4">{@render examples()}</div>
</Story>
<Story name="Visual composer" asChild>
  <div class="max-w-xl bg-background p-4">
    <TipTapEditor
      onReady={(editor) =>
        editor.setContent(
          `Please join ${longLink} or [read the guide](https://docs.example.com/guide).`
        )}
    />
  </div>
</Story>
