<script lang="ts">
  import ServerMarkdown from '$lib/components/ServerMarkdown.svelte';
  import { m } from '$lib/i18n/messages';
  import { Dialog } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  let { motd, onclose }: { motd: string; onclose: () => void } = $props();
  let visible = $state(true);
</script>

<Dialog bind:visible title={m('common.message_of_the_day')} {onclose}>
  <ServerMarkdown source={motd} />
  {#snippet dismissAction()}
    <Button variant="secondary" defaultAction onclick={() => (visible = false)}>
      {m('ui.close')}
    </Button>
  {/snippet}
</Dialog>
