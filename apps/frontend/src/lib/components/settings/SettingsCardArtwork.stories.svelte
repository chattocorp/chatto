<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { resolve } from '$app/paths';
  import SettingsCardArtwork from './SettingsCardArtwork.svelte';

  const { Story } = defineMeta({
    title: 'Settings/Overview artwork',
    component: SettingsCardArtwork
  });
</script>

<Story name="Updates" asChild>
  <a
    href={resolve('/chat/preferences/desktop')}
    class="group hover-lift-group block w-56 max-w-full cursor-pointer rounded-lg focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-action"
  >
    <div class="flex hover-lift flex-col inset-frame">
      <div class="overflow-hidden inset-content">
        <SettingsCardArtwork href="/chat/preferences/desktop" icon="iconify icon-[uil--desktop]" />
      </div>
      <div class="flex flex-col gap-2 p-3">
        <span class="font-bold text-text-top">Updates</span>
        <p class="min-h-[3lh] text-sm text-muted">Updates for this installation</p>
      </div>
    </div>
  </a>
</Story>
