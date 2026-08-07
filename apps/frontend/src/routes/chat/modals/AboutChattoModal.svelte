<script module lang="ts">
  let simulatedChattoWordmarkModule: Promise<
    typeof import('$lib/components/SimulatedChattoWordmark.svelte')
  > | null = null;

  function loadSimulatedChattoWordmark() {
    simulatedChattoWordmarkModule ??= import('$lib/components/SimulatedChattoWordmark.svelte');
    return simulatedChattoWordmarkModule;
  }
</script>

<script lang="ts">
  import { version } from '$app/environment';
  import { m } from '$lib/i18n/messages';
  import Dialog from '$lib/ui/Dialog.svelte';

  let {
    onclose
  }: {
    onclose: () => void;
  } = $props();
</script>

<Dialog visible title={m('ui.tooltip.about', { subject: 'Chatto' })} size="lg" {onclose}>
  <div class="flex flex-col items-center gap-4 text-sm">
    <div class="flex aspect-[2/1] w-full items-center justify-center">
      {#await loadSimulatedChattoWordmark() then { default: SimulatedChattoWordmark }}
        <SimulatedChattoWordmark contained />
      {/await}
    </div>

    <p class="text-muted tabular-nums">v{version}</p>

    <div class="flex flex-wrap items-center justify-center gap-x-5 gap-y-2">
      <a
        href="https://github.com/chattocorp/chatto"
        target="_blank"
        rel="noopener noreferrer"
        class="inline-flex items-center gap-1.5 link"
      >
        <span class="iconify icon-[mdi--github] text-base" aria-hidden="true"></span>
        <span>github.com/chattocorp/chatto</span>
        <span class="iconify icon-[mdi--open-in-new] text-sm" aria-hidden="true"></span>
      </a>
      <a
        href="https://docs.chatto.run"
        target="_blank"
        rel="noopener noreferrer"
        class="inline-flex items-center gap-1.5 link"
      >
        <span
          class="iconify icon-[mdi--book-open-page-variant-outline] text-base"
          aria-hidden="true"
        ></span>
        <span>docs.chatto.run</span>
        <span class="iconify icon-[mdi--open-in-new] text-sm" aria-hidden="true"></span>
      </a>
    </div>
  </div>
</Dialog>
