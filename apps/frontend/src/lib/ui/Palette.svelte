<!--
@component

Shared shell for compact app-wide palettes such as Quick Finder and the
notification centre. Palette owns the frame, responsive presentation, backdrop,
and dismissal behavior so consumers cannot drift into separate visual systems.

- `modal` centres the palette near the top of the desktop viewport.
- `anchored` places it below a desktop trigger.
- Both presentations use the same bottom sheet on mobile.

Consumers own only the palette's inset sections and their content.
-->
<script lang="ts">
  import { onDestroy, type Snippet } from 'svelte';
  import { sidebarNav } from '$lib/state/globals.svelte';
  import BottomSheet from './BottomSheet.svelte';
  import FloatingPopover from './FloatingPopover.svelte';

  let {
    visible,
    presentation = 'modal',
    anchor,
    id,
    ariaLabel,
    onclose,
    onopen,
    onclosed,
    children
  }: {
    visible: boolean;
    presentation?: 'modal' | 'anchored';
    anchor?: { top: number; bottom: number; left: number } | null;
    id?: string;
    ariaLabel: string;
    onclose: () => void;
    onopen?: () => void;
    onclosed?: () => void;
    children: Snippet;
  } = $props();

  let dialogEl: HTMLDialogElement | undefined;
  let lifecycleVisible = false;

  // A responsive desktop/mobile presentation change must not end the logical
  // palette session. Drive consumer lifecycle from visibility, not from the
  // currently mounted surface branch.
  $effect(() => {
    if (visible === lifecycleVisible) return;
    lifecycleVisible = visible;
    if (visible) onopen?.();
    else onclosed?.();
  });

  onDestroy(() => {
    if (lifecycleVisible) onclosed?.();
  });

  function syncModal(node: HTMLDialogElement) {
    dialogEl = node;
    if (visible && !node.open) node.showModal();
    else if (!visible && node.open) node.close();
  }
</script>

<svelte:window
  onkeydown={(event) => {
    if (event.key === 'Escape' && visible && presentation === 'anchored' && !sidebarNav.isMobile) {
      onclose();
    }
  }}
  onresize={() => {
    if (visible && presentation === 'anchored' && !sidebarNav.isMobile) onclose();
  }}
/>

{#snippet surface()}
  <div {id} class="flex w-140 max-w-[90vw] flex-col gap-1 menu">
    {@render children()}
  </div>
{/snippet}

{#if sidebarNav.isMobile && visible}
  <BottomSheet visible {ariaLabel} {onclose}>
    <div class="flex justify-center overflow-hidden">
      {@render surface()}
    </div>
  </BottomSheet>
{:else if presentation === 'anchored' && visible && anchor}
  <FloatingPopover {anchor} role="dialog" {ariaLabel} {onclose} class="bg-transparent p-0">
    {@render surface()}
  </FloatingPopover>
{:else if presentation === 'modal'}
  <dialog
    {@attach syncModal}
    onclose={() => {
      if (visible) onclose();
    }}
    onkeydown={(event) => {
      if (event.key === 'Escape') event.stopPropagation();
    }}
    oncancel={(event) => {
      event.preventDefault();
      onclose();
    }}
    onclick={(event) => {
      if (event.target === dialogEl) onclose();
    }}
    aria-label={ariaLabel}
    class="palette-dialog m-auto mt-[15vh] max-h-none max-w-none overflow-visible border-none bg-transparent p-0 text-inherit backdrop:bg-black/50"
  >
    {#if visible}{@render surface()}{/if}
  </dialog>
{/if}
