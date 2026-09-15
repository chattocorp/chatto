<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CallDeviceSettings from './CallDeviceSettings.svelte';
  import CallDeviceSettingsStoryHarness from './CallDeviceSettingsStoryHarness.svelte';
  const { Story } = defineMeta({
    title: 'Settings/Voice and video',
    component: CallDeviceSettings,
    tags: ['autodocs']
  });
</script>

<Story name="Default devices" asChild>
  <CallDeviceSettingsStoryHarness />
</Story>
<Story name="Saved device unavailable" asChild>
  <CallDeviceSettingsStoryHarness unavailable />
</Story>
<Story name="During a call" asChild>
  <CallDeviceSettingsStoryHarness inCall />
</Story>
