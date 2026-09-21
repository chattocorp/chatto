<script lang="ts">
  import { onMount } from "svelte";
  import ActivityIndicator from "./ActivityIndicator.svelte";

  let dialog: HTMLDialogElement;
  onMount(() => {
    // A modal also blocks other open dialogs, including the run composer.
    dialog.showModal();
    return () => dialog.close();
  });
</script>

<dialog
  bind:this={dialog}
  class="fixed inset-0 m-0 h-dvh max-h-none w-screen max-w-none border-0 bg-transparent p-0 text-base-content outline-none backdrop:bg-base-100/40"
  aria-labelledby="reconnecting-title"
  aria-describedby="reconnecting-description"
  oncancel={(event) => event.preventDefault()}
  onkeydown={(event) => event.stopPropagation()}
>
  <div class="absolute inset-x-4 bottom-6 mx-auto flex w-fit max-w-full items-center gap-3 rounded-lg border border-base-300 bg-base-200 px-4 py-3 shadow-lg" role="status">
    <span class="text-primary"><ActivityIndicator /></span>
    <div>
      <p id="reconnecting-title" class="text-sm font-medium">Reconnecting…</p>
      <p id="reconnecting-description" class="mt-0.5 text-xs text-base-content/60">Controls will return when the connection is restored.</p>
    </div>
  </div>
</dialog>
