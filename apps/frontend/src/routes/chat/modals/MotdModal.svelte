<script lang="ts">
  import MarkdownContent from '$lib/components/MarkdownContent.svelte';
  import { m } from '$lib/i18n/messages';
  import { Dialog } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  let { motd, onclose }: { motd: string; onclose: () => void } = $props();
  let visible = $state(true);
</script>

<Dialog bind:visible title={m('common.message_of_the_day')} {onclose}>
  <MarkdownContent source={motd} />
  {#snippet dismissAction()}
    <Button variant="secondary" defaultAction onclick={() => (visible = false)}>
      {m('ui.close')}
    </Button>
  {/snippet}
</Dialog>
