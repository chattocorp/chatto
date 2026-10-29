<!-- @component Shows the registered servers that supplied a recommendation, without contacting the recommended server. -->
<script lang="ts">
  import type { ChatModal } from '$lib/modal';
  import { m } from '$lib/i18n/messages';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import { Dialog } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { serverHost } from '$lib/serverUrl';

  let {
    modal,
    onclose
  }: { modal: Extract<ChatModal, { type: 'serverRecommendation' }>; onclose: () => void } =
    $props();
  let visible = $state(true);
  const sourceNames = $derived(
    new Intl.ListFormat(getReactiveLocale(), { style: 'long', type: 'conjunction' }).format(
      modal.sources.map((source) => source.name)
    )
  );
</script>

<Dialog bind:visible title={m('ui.tooltip.about', { subject: modal.name })} {onclose}>
  <div class="flex flex-col gap-4">
    <p class="text-muted" dir="ltr">{serverHost(modal.origin)}</p>
    <p class="text-text">
      <bdi>{m('add_server.directory.recommended_by', { servers: sourceNames })}</bdi>
    </p>
    <ul class="flex flex-col gap-3">
      {#each modal.sources as source (source.origin)}
        <li class="min-w-0">
          <p class="font-semibold text-text"><bdi>{source.name}</bdi></p>
          <p class="wrap-anywhere text-muted" dir="ltr">{serverHost(source.origin)}</p>
        </li>
      {/each}
    </ul>
  </div>
  {#snippet dismissAction()}
    <Button variant="secondary" defaultAction onclick={() => (visible = false)}
      >{m('ui.close')}</Button
    >
  {/snippet}
</Dialog>
