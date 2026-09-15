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

<Story name="Sensitivity during a call" asChild>
  <CallDeviceSettingsStoryHarness inCall threshold={-30} />
</Story>
<Story name="Sensitivity unavailable" asChild>
  <CallDeviceSettingsStoryHarness inCall threshold={-30} gateUnavailable />
</Story>
