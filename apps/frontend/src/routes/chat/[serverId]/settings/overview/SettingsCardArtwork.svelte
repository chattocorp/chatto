<!-- @component Decorative miniature of a settings destination. The card supplies its accessible name. -->
<script lang="ts">
  import SettingsDestinationArtwork from './SettingsDestinationArtwork.svelte';

  let { href, icon }: { href: string; icon: string } = $props();
  const kind = $derived(href.split('/').at(-1));
</script>

<div
  aria-hidden="true"
  class="relative flex h-28 items-center justify-center overflow-hidden bg-action/5 text-action"
>
  <svg
    viewBox="0 0 480 176"
    class="absolute inset-0 size-full"
    preserveAspectRatio="xMidYMid slice"
  >
    <circle cx="390" cy="12" r="150" fill="currentColor" opacity="0.04" />
    <circle cx="390" cy="12" r="110" fill="none" stroke="currentColor" stroke-opacity="0.08" />
    <circle cx="65" cy="190" r="100" fill="none" stroke="currentColor" stroke-opacity="0.08" />
  </svg>

  <div class="flex scale-75 items-center justify-center">
    {#if kind === 'appearance'}
      <div class="relative flex items-center -space-x-8">
        <div
          class="h-28 w-40 -rotate-6 overflow-hidden rounded-xl border border-border bg-background shadow-lg transition-transform duration-200 group-hover:-rotate-3 motion-reduce:transition-none"
        >
          <div class="flex h-6 items-center gap-1 border-b border-border px-3">
            <span class="size-1.5 rounded-full bg-action"></span><span
              class="size-1.5 rounded-full bg-action/30"
            ></span><span class="size-1.5 rounded-full bg-action/15"></span>
          </div>
          <div class="flex h-full gap-3 p-3">
            <div class="w-7 rounded bg-action/10"></div>
            <div class="flex flex-1 flex-col gap-2">
              <div class="h-3 w-12 rounded bg-action/40"></div>
              <div class="h-2 rounded bg-text/10"></div>
              <div class="h-2 w-3/4 rounded bg-text/10"></div>
            </div>
          </div>
        </div>
        <div
          class="relative mt-5 flex h-24 w-36 rotate-6 flex-col justify-between rounded-xl border border-border bg-surface-emphasized p-4 shadow-lg transition-transform duration-200 group-hover:rotate-3 motion-reduce:transition-none"
        >
          <span aria-hidden="true" class="iconify icon-[uil--palette] text-2xl"></span>
          <div class="flex gap-2">
            <span class="size-5 rounded-full bg-action"></span><span
              class="size-5 rounded-full bg-success"
            ></span><span class="size-5 rounded-full bg-warning"></span><span
              class="size-5 rounded-full bg-danger"
            ></span>
          </div>
        </div>
      </div>
    {:else if kind === 'language'}
      <div class="relative flex items-center -space-x-3">
        <span
          class="flex size-20 -rotate-12 items-center justify-center rounded-2xl border border-border bg-background text-5xl font-semibold shadow-lg transition-transform duration-200 group-hover:-rotate-6 motion-reduce:transition-none"
          >A</span
        >
        <span
          class="z-10 -mt-5 flex size-24 items-center justify-center rounded-2xl bg-action text-5xl font-semibold text-background shadow-lg"
          >あ</span
        >
        <span
          class="mt-6 flex size-20 rotate-12 items-center justify-center rounded-2xl border border-border bg-surface-emphasized text-4xl shadow-lg transition-transform duration-200 group-hover:rotate-6 motion-reduce:transition-none"
          >文</span
        >
      </div>
    {:else if kind === 'composer'}
      <div
        class="relative w-60 -rotate-3 rounded-xl border border-border bg-background p-4 shadow-lg transition-transform duration-200 group-hover:rotate-0 motion-reduce:transition-none"
      >
        <div class="mb-4 flex gap-3 border-b border-border pb-3 text-muted">
          <span aria-hidden="true" class="iconify icon-[uil--bold] text-lg"></span><span
            aria-hidden="true"
            class="iconify icon-[uil--italic] text-lg"
          ></span><span aria-hidden="true" class="iconify icon-[uil--link] text-lg"></span><span
            aria-hidden="true"
            class="iconify icon-[uil--list-ul] text-lg"
          ></span>
        </div>
        <div class="flex flex-col gap-2">
          <span class="h-2 w-4/5 rounded-full bg-text/20"></span><span
            class="h-2 w-3/5 rounded-full bg-text/10"
          ></span><span class="h-2 w-2/5 rounded-full bg-action/40"></span>
        </div>
        <span
          class="absolute -end-4 -bottom-3 flex size-12 rotate-3 items-center justify-center rounded-2xl bg-action text-background shadow-lg"
          ><span aria-hidden="true" class="iconify icon-[uil--message] text-2xl rtl:-scale-x-100"
          ></span></span
        >
      </div>
    {:else if kind === 'voice'}
      <div class="relative flex items-center gap-2">
        {#each [24, 48, 72, 40] as height, index (index)}
          <span class="w-2 rounded-full bg-action/25" style:height={`${height}px`}></span>
        {/each}
        <span
          class="mx-4 flex size-24 items-center justify-center rounded-full border border-action/20 bg-background text-5xl shadow-lg"
          ><span aria-hidden="true" class={icon}></span></span
        >
        {#each [56, 80, 40, 24] as height, index (index)}
          <span class="w-2 rounded-full bg-action/25" style:height={`${height}px`}></span>
        {/each}
      </div>
    {:else}
      <SettingsDestinationArtwork {kind} />
    {/if}
  </div>
</div>
