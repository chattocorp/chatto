<!--
@component

Desktop game-capture source picker. The host supplies temporary opaque source
IDs; this component only presents window metadata and reports explicit choice.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type { GameCaptureSource } from '$lib/desktop/gameCapture';
  import Dialog from '$lib/ui/Dialog.svelte';
  import { Button } from '$lib/ui/form';

  let {
    visible = $bindable(false),
    sources,
    loading,
    failed,
    onretry,
    onselect
  }: {
    visible?: boolean;
    sources: GameCaptureSource[];
    loading: boolean;
    failed: boolean;
    onretry: () => void;
    onselect: (source: GameCaptureSource) => void;
  } = $props();

  const id = $props.id();
  const descriptionId = `${id}-description`;
</script>

<Dialog bind:visible title={m('voice.stream_game')} size="md" describedBy={descriptionId}>
  <p id={descriptionId} class="text-muted">
    {m('voice.stream_game_description')}
  </p>

  {#if loading}
    <div class="flex min-h-32 items-center justify-center gap-3 text-muted" role="status">
      <span class="iconify icon-[uil--spinner] animate-spin text-xl" aria-hidden="true"></span>
      <span>{m('common.loading')}</span>
    </div>
  {:else if failed}
    <div class="flex min-h-32 flex-col items-center justify-center gap-4 text-center" role="alert">
      <p class="text-muted">{m('voice.game_windows_failed')}</p>
      <Button variant="secondary" onclick={onretry}>{m('common.retry')}</Button>
    </div>
  {:else if sources.length === 0}
    <p class="flex min-h-32 items-center justify-center text-center text-muted">
      {m('voice.no_game_windows')}
    </p>
  {:else}
    <ul class="mt-4 selectable-list" aria-label={m('voice.game_windows')}>
      {#each sources as source (source.id)}
        <li>
          <button
            type="button"
            class="flex w-full cursor-pointer items-center gap-3 selectable-list-item p-3 text-start"
            onclick={() => onselect(source)}
          >
            <span class="iconify icon-[uil--window] shrink-0 text-xl text-muted" aria-hidden="true"
            ></span>
            <span class="flex min-w-0 flex-1 flex-col">
              <span class="truncate font-medium"><bdi>{source.applicationName}</bdi></span>
              <span class="truncate text-muted">
                <bdi>{source.title || m('voice.untitled_window')}</bdi>
              </span>
            </span>
            <span class="shrink-0 text-muted tabular-nums">
              {source.width}×{source.height}
            </span>
            <span
              class="iconify icon-[uil--angle-right] shrink-0 text-xl text-muted rtl:rotate-180"
              aria-hidden="true"
            ></span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</Dialog>
