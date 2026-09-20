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

<Story name="Touch-friendly actions" asChild>
  <div class="w-80 shell-surface p-2">
    <div class="flex gap-1">
      <CompactActionButton label="Formatting" touchFriendly>Aa</CompactActionButton>
      <CompactActionButton label="Attach file" touchFriendly>
        <span class="iconify icon-[uil--image-upload]" aria-hidden="true"></span>
      </CompactActionButton>
      <CompactActionButton label="Send" touchFriendly disabled>
        <span class="iconify icon-[uil--telegram-alt]" aria-hidden="true"></span>
      </CompactActionButton>
    </div>
  </div>
</Story>
