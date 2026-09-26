<script lang="ts">
  import Dialog from './Dialog.svelte';
  import { Button } from './form';
  let {
    longBody = false,
    loading = false,
    onclose = () => {}
  }: {
    longBody?: boolean;
    loading?: boolean;
    onclose?: () => void;
  } = $props();
</script>

<Dialog visible title="Im vorherigen Thread fortfahren?" {onclose}>
  <label for="dialog-draft">Nachricht</label>
  <textarea id="dialog-draft" class="w-full"></textarea>
  {#if longBody}
    {#each Array(30) as _, index (index)}<p>Nachricht {index + 1}: Bitte prüfe den Text.</p>{/each}
  {/if}
  {#snippet primaryAction()}
    <Button
      defaultAction
      {loading}
      loadingText="Deine Nachricht wird im vorherigen Diskussionsthread gesendet"
    >
      <span class="iconify icon-[uil--comment-alt-lines]"></span>Im Thread fortfahren
    </Button>
  {/snippet}
  {#snippet secondaryActions()}
    <Button variant="secondary">Als neue Nachricht senden</Button>
  {/snippet}
  {#snippet dismissAction()}
    <Button variant="secondary" onclick={onclose}>Abbrechen</Button>
  {/snippet}
</Dialog>
