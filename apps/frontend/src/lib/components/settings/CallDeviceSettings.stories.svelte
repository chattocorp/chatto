<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { spyOn } from 'storybook/test';
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
<Story
  name="Many devices and long labels"
  beforeEach={() => {
    const devices = (['audioinput', 'audiooutput', 'videoinput'] as const).flatMap((kind) =>
      Array.from({ length: 20 }, (_, index) => ({
        kind,
        deviceId: `${kind}-${index}`,
        groupId: 'storybook',
        label: `Studio ${kind} ${index + 1} — Virtual conference and recording device with a long name`,
        toJSON() {
          return {};
        }
      }))
    );
    const enumerate = spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue(devices);
    return () => enumerate.mockRestore();
  }}
  asChild
>
  <CallDeviceSettingsStoryHarness inCall />
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

<Story name="Voice boosting off" asChild>
  <CallDeviceSettingsStoryHarness inCall threshold={-35} effects={false} />
</Story>
