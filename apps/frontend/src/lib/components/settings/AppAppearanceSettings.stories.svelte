<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AppAppearanceSettings from './AppAppearanceSettings.svelte';
  const { Story } = defineMeta({
    title: 'Settings/Appearance',
    component: AppAppearanceSettings,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import { onMount } from 'svelte';
  import { userPreferences, applyAccentColor, applySurfaceDepth } from '$lib/state/userPreferences.svelte';

  // The app's HTML bootstrap owns this at runtime; Storybook has its own shell.
  onMount(() => {
    const previousDepth = document.documentElement.dataset.depth;
    applySurfaceDepth(userPreferences.surfaceDepth);
    const previous = document.documentElement.dataset.accent;
    applyAccentColor(userPreferences.accentColor);
    return () => {
      if (previousDepth) document.documentElement.dataset.depth = previousDepth;
      else delete document.documentElement.dataset.depth;
      if (previous) document.documentElement.dataset.accent = previous;
      else delete document.documentElement.dataset.accent;
    };
  });
</script>

<Story name="Appearance" asChild>
  <div class="pane-page h-[850px] w-full">
    <AppAppearanceSettings />
  </div>
</Story>
