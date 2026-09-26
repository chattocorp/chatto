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
  import {
    userPreferences,
    applyAccentColor,
    applyContrastAge,
    applySurfaceDepth
  } from '$lib/state/userPreferences.svelte';

  // The app's HTML bootstrap owns this at runtime; Storybook has its own shell.
  onMount(() => {
    const previousDepth = document.documentElement.style.getPropertyValue('--depth-level');
    applySurfaceDepth(userPreferences.surfaceDepth);
    const previous = document.documentElement.dataset.accent;
    applyAccentColor(userPreferences.accentColor);
    const previousSoft = document.documentElement.style.getPropertyValue('--contrast-soft-mix');
    const previousStrong = document.documentElement.style.getPropertyValue('--contrast-strong-mix');
    applyContrastAge(userPreferences.contrastAge);
    return () => {
      document.documentElement.style.setProperty('--depth-level', previousDepth);
      if (previous) document.documentElement.dataset.accent = previous;
      else delete document.documentElement.dataset.accent;
      document.documentElement.style.setProperty('--contrast-soft-mix', previousSoft);
      document.documentElement.style.setProperty('--contrast-strong-mix', previousStrong);
    };
  });
</script>

<Story name="Appearance" asChild>
  <div class="pane-page h-[1000px] w-full">
    <AppAppearanceSettings />
  </div>
</Story>
