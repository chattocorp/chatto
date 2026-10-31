<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import UrlLink from './UrlLink.svelte';
  import TipTapEditor from '../composer/TipTapEditor.svelte';

  const { Story } = defineMeta({ title: 'Messages/URL chips', component: UrlLink });
  const longLink = 'http://chatto.ws55110.localhost:55110/chat/-/user/UT9SQd0NQaXj3pc';
</script>

{#snippet examples()}
  <p>
    Release: <UrlLink
      href="https://www.alchemy-cms.com/news/alchemy-8-4-released"
      title="Alchemy 8.4 released"
    />.
  </p>
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
  <p>Loading metadata: <UrlLink href="https://example.com/guide" loading />.</p>
  <p>
    Long title: <UrlLink
      href="https://example.com/guide"
      title="A comprehensive guide to setting up your server and welcoming your community"
    />.
  </p>
  <p>
    Article: <UrlLink
      href="https://spiegel.de/politik/deutschland/ibrahim-ibrahim"
      title="(S+) Ibrahim Ibrahim: Wer ist der Mann, vor dem Elif Eralp Angst hat?"
    />.
  </p>
  <p>
    Long source: <UrlLink
      href="https://a-very-long-subdomain.another-long-subdomain.example.com:8443/guide"
      title="A guide from a community server"
    />.
  </p>
{/snippet}

<Story name="Automatic web links" globals={{ theme: 'light' }} asChild>
  <div class="prose max-w-xl bg-background p-4">{@render examples()}</div>
</Story>
<Story name="Dark" globals={{ theme: 'dark' }} asChild>
  <div class="prose max-w-xl bg-background p-4">{@render examples()}</div>
</Story>
<Story name="Narrow message" asChild>
  <div class="prose w-64 max-w-full bg-background p-4">{@render examples()}</div>
</Story>
<Story name="Right to left" asChild>
  <div class="prose w-80 max-w-full bg-background p-4" dir="rtl">{@render examples()}</div>
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
