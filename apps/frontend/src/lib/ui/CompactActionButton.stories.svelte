<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CompactActionButton from './CompactActionButton.svelte';
  const { Story } = defineMeta({ title: 'UI/CompactActionButton', component: CompactActionButton });
</script>

<Story name="Actions on a surface" asChild>
  <div class="flex gap-1 shell-surface p-2">
    <CompactActionButton label="Formatting" class="text-sm font-semibold">Aa</CompactActionButton>
    <CompactActionButton label="Mute">
      <span class="iconify icon-[uil--volume-up]" aria-hidden="true"></span>
    </CompactActionButton>
    <CompactActionButton label="Fullscreen">
      <span class="iconify icon-[mdi--fullscreen]" aria-hidden="true"></span>
    </CompactActionButton>
    <CompactActionButton label="Send" disabled>
      <span class="iconify icon-[uil--telegram-alt]" aria-hidden="true"></span>
    </CompactActionButton>
  </div>
</Story>
