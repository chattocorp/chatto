<!--
@component

Inline Markdown for short, server-supplied text such as the server description
and the message of the day. It renders formatting and safe links, but no block
markup other than paragraphs. The plain text shows while the Markdown renderer loads, and when it
fails to load.
-->
<script lang="ts">
  import MarkdownHtml from './MarkdownHtml.svelte';

  let {
    text,
    multiline = false
  }: {
    text: string;
    /**
     * Renders paragraphs and line breaks of `text`. The default shows all text
     * on one line.
     */
    multiline?: boolean;
  } = $props();

  const html = $derived.by(() => {
    const body = text;
    const options = { multiline };
    return import('$lib/markdown').then(({ renderInlineMarkdown }) =>
      renderInlineMarkdown(body, options)
    );
  });
</script>

{#await html}
  {text}
{:then html}
  <MarkdownHtml {html} />
{:catch}
  {text}
{/await}
