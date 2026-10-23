<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { Dialog, MarkdownHtml } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  let { motd, onclose }: { motd: string; onclose: () => void } = $props();
  let visible = $state(true);

  const html = $derived.by(() => {
    const body = motd;
    return import('$lib/markdown').then(({ renderMarkdown }) => renderMarkdown(body));
  });
</script>

<Dialog bind:visible title={m('common.message_of_the_day')} {onclose}>
  <div class="prose max-w-none wrap-anywhere" dir="auto">
    {#await html}
      <p class="whitespace-pre-wrap">{motd}</p>
    {:then html}
      <MarkdownHtml {html} />
    {:catch}
      <p class="whitespace-pre-wrap">{motd}</p>
    {/await}
  </div>
  {#snippet dismissAction()}
    <Button variant="secondary" defaultAction onclick={() => (visible = false)}>
      {m('ui.close')}
    </Button>
  {/snippet}
</Dialog>
