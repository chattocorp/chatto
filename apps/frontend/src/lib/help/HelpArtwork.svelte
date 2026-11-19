<!-- @component Small, decorative examples of the interface taught by each guide. -->
<script lang="ts">
  import { helpTopicIcons, type HelpTopic } from './navigation';
  let { topic }: { topic: HelpTopic } = $props();
</script>

<div
  class="flex h-32 items-center justify-center overflow-hidden bg-action/5 p-4 text-action sm:h-40"
>
  <span class={[helpTopicIcons[topic], 'text-4xl sm:hidden']} aria-hidden="true"></span>
  <div class="hidden sm:contents">
    {#if topic === 'conversations'}
      <div class="flex w-52 flex-col gap-3">
        <div class="flex gap-2">
          <span class="size-7 shrink-0 rounded-full bg-action/25"></span>
          <div class="flex-1 rounded-lg bg-surface p-3">
            <div class="h-2 w-16 rounded bg-text/35"></div>
            <div class="mt-2 h-2 w-28 rounded bg-text/10"></div>
          </div>
        </div>
        <div class="ms-9 flex gap-2">
          <span class="size-6 shrink-0 rounded-full bg-success/25"></span>
          <div class="flex-1 rounded-lg bg-surface p-3">
            <div class="h-2 w-20 rounded bg-text/20"></div>
          </div>
        </div>
      </div>
    {:else if topic === 'formatting'}
      <div class="flex w-52 flex-col gap-4 rounded-lg bg-surface p-4 text-text">
        <div class="flex items-center gap-5 border-b border-border pb-3">
          <b class="text-xl">B</b><i class="text-xl">I</i><span
            class="iconify icon-[uil--link]"
            aria-hidden="true"
          ></span><code class="text-action">&lt;/&gt;</code>
        </div>
        <div class="h-2 w-32 rounded bg-text/25"></div>
        <div class="h-2 w-24 rounded bg-text/10"></div>
      </div>
    {:else if topic === 'bots'}
      <span class="iconify icon-[uil--robot] text-7xl" aria-hidden="true"></span>
    {:else}
      <span class="iconify icon-[uil--mobile-android] text-7xl" aria-hidden="true"></span>
    {/if}
  </div>
</div>
