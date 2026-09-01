<!--
@component

Desktop-native screen-share source picker. The host supplies temporary opaque
source IDs and in-memory JPEG previews; this component revokes every preview
URL when its source changes or leaves the DOM.
-->
<script lang="ts">
  import type { Attachment } from 'svelte/attachments';
  import { m } from '$lib/i18n/messages';
  import type { NativeScreenShareSource } from '$lib/desktop/nativeScreenShare';
  import { Dialog } from '$lib/ui';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import SegmentedControl from '$lib/ui/SegmentedControl.svelte';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { Button } from '$lib/ui/form';

  let {
    visible = $bindable(false),
    sources,
    loading,
    failed,
    onretry,
    onselect,
    ondismiss = () => {}
  }: {
    visible?: boolean;
    sources: NativeScreenShareSource[];
    loading: boolean;
    failed: boolean;
    onretry: () => void;
    onselect: (source: NativeScreenShareSource) => void;
    ondismiss?: () => void;
  } = $props();

  type SourceKind = NativeScreenShareSource['kind'];
  let sourceKind = $state<SourceKind>('window');
  let selectionMade = false;
  let visibleSources = $derived(sources.filter((source) => source.kind === sourceKind));
  let sourceKindOptions = $derived([
    { value: 'window' as const, label: m('voice.applications') },
    { value: 'display' as const, label: m('voice.entire_screen') }
  ]);

  function close() {
    visible = false;
  }

  function handleClose() {
    sourceKind = 'window';
    if (selectionMade) {
      selectionMade = false;
      return;
    }
    ondismiss();
  }

  function select(source: NativeScreenShareSource) {
    selectionMade = true;
    visible = false;
    onselect(source);
  }

  function previewImage(preview: Uint8Array): Attachment<HTMLImageElement> {
    return (image) => {
      const bytes = new Uint8Array(preview.byteLength);
      bytes.set(preview);
      const url = URL.createObjectURL(new Blob([bytes.buffer], { type: 'image/jpeg' }));
      image.src = url;
      return () => URL.revokeObjectURL(url);
    };
  }

  function sourceLabel(source: NativeScreenShareSource): string {
    if (source.kind === 'window') return source.title || source.applicationName;
    return source.isMainDisplay
      ? m('voice.main_display')
      : m('voice.display_number', { number: source.displayIndex });
  }
</script>

{#snippet footer()}
  <Button variant="secondary" onclick={close}>{m('common.cancel')}</Button>
{/snippet}

<Dialog bind:visible title={m('voice.share_screen')} size="lg" onclose={handleClose} {footer}>
  <div class="flex justify-center">
    <SegmentedControl
      label={m('voice.share_source')}
      options={sourceKindOptions}
      value={sourceKind}
      onchange={(value) => (sourceKind = value)}
      class="w-full sm:w-auto"
    />
  </div>

  {#if loading}
    <div
      class="mt-4 grid max-h-[52vh] grid-cols-1 gap-3 overflow-y-auto rounded-lg bg-background p-3 sm:grid-cols-2"
      aria-label={m('common.loading')}
      role="status"
    >
      {#each Array(4) as _, index (index)}
        <div class="overflow-hidden rounded-md bg-surface">
          <div class="skeleton aspect-video"></div>
          <div class="flex flex-col gap-2 p-3">
            <div class="skeleton h-4 w-2/3 rounded"></div>
            <div class="skeleton h-4 w-1/3 rounded"></div>
          </div>
        </div>
      {/each}
    </div>
  {:else if failed}
    <div class="mt-4 flex min-h-64 flex-col rounded-lg bg-background" role="alert">
      <EmptyState icon="icon-[uil--exclamation-circle]" title={m('voice.share_sources_failed')}>
        <Button variant="secondary" size="sm" onclick={onretry}>{m('common.retry')}</Button>
      </EmptyState>
    </div>
  {:else if visibleSources.length === 0}
    <div class="mt-4 flex min-h-64 flex-col rounded-lg bg-background">
      <EmptyState icon="icon-[uil--window-section]">
        {sourceKind === 'window' ? m('voice.no_share_windows') : m('voice.no_share_displays')}
      </EmptyState>
    </div>
  {:else}
    <ul
      class="mt-4 grid max-h-[52vh] grid-cols-1 gap-3 overflow-y-auto rounded-lg bg-background p-3 sm:grid-cols-2"
      aria-label={sourceKind === 'window' ? m('voice.applications') : m('voice.entire_screen')}
    >
      {#each visibleSources as source (source.id)}
        <li class="min-w-0">
          <button
            type="button"
            class="group w-full cursor-pointer overflow-hidden rounded-md border border-input bg-surface text-start transition-[background-color,border-color] hover:border-action hover:bg-surface-emphasized focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action"
            onclick={() => select(source)}
          >
            <span
              class="grid aspect-video w-full place-items-center overflow-hidden bg-black text-white/70"
            >
              {#if source.preview.byteLength > 0}
                <SkeletonImg
                  alt=""
                  class="size-full object-contain"
                  {@attach previewImage(source.preview)}
                />
              {:else}
                <span
                  class={[
                    'iconify text-4xl',
                    source.kind === 'window' ? 'icon-[uil--window]' : 'icon-[uil--desktop]'
                  ]}
                  aria-hidden="true"
                ></span>
              {/if}
            </span>
            <span class="flex min-w-0 flex-col gap-1 p-3">
              <span class="truncate font-medium text-text-top">
                <bdi>{sourceLabel(source)}</bdi>
              </span>
              <span class="flex min-w-0 justify-between gap-2 text-sm text-muted">
                {#if source.kind === 'window'}
                  <bdi class="truncate">{source.applicationName}</bdi>
                {:else}
                  <span>{m('voice.video_only')}</span>
                {/if}
                <span class="shrink-0 tabular-nums">{source.width}×{source.height}</span>
              </span>
            </span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</Dialog>
