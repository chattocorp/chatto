<script lang="ts">
  import * as m from '$lib/i18n/messages';

  let {
    visible = false,
    title = m['composer.drop_files_title'](),
    subtitle = m['composer.drop_files_subtitle']()
  }: { visible: boolean; title?: string; subtitle?: string } = $props();
</script>

{#if visible}
  <div
    class="pointer-events-none absolute inset-0 z-50 flex items-center justify-center rounded-[inherit] bg-neutral-action/10 backdrop-blur-xs"
  >
    <div
      class="flex flex-col items-center gap-3 rounded-xl border-2 border-dashed border-neutral-action bg-surface/90 px-8 py-6 shadow-lg"
    >
      <span class="iconify text-5xl text-neutral-action uil--image-upload"></span>
      <p class="text-lg font-medium text-text">{title}</p>
      <p class="text-sm text-muted">{subtitle}</p>
    </div>
  </div>
{/if}
