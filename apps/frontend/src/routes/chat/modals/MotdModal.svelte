<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import Dialog from '$lib/ui/Dialog.svelte';
  import MarkdownHtml from '$lib/ui/MarkdownHtml.svelte';
  import Button from '$lib/ui/form/Button.svelte';

  let { motd, onclose }: { motd: string; onclose: () => void } = $props();
  let visible = $state(true);

  const html = $derived(import('$lib/markdown').then(({ renderMarkdown }) => renderMarkdown(motd)));
</script>

<Dialog bind:visible title={m('server_settings.motd_label')} {onclose}>
  <div class="prose max-w-none wrap-anywhere" dir="auto">
    {#await html}
      <p class="whitespace-pre-wrap">{motd}</p>
    {:then html}
      <MarkdownHtml {html} />
    {:catch}
      <p class="whitespace-pre-wrap">{motd}</p>
    {/await}
  </div>
  {#snippet footer()}
    <Button variant="secondary" defaultAction onclick={() => (visible = false)}>
      {m('ui.close')}
    </Button>
  {/snippet}
</Dialog>
