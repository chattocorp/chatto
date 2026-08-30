<!--
@component

Floating context menu for selecting audio input (microphone), output (speaker),
and video input (camera) devices.
Reads available devices and current selection from `voiceCallState`.

**Props:**
- `anchor` - Position rect for the ContextMenu
- `onclose` - Called when the menu should dismiss
-->
<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import MenuItem from '$lib/ui/MenuItem.svelte';
  import MenuSection from '$lib/ui/MenuSection.svelte';

  const serverScope = useServerScope();
  const voiceCallState = $derived(serverScope.store.voiceCall);

  let {
    anchor,
    onclose
  }: {
    anchor: { top: number; bottom: number; left: number };
    onclose: () => void;
  } = $props();

  type DeviceSection = {
    label: string;
    devices: MediaDeviceInfo[];
    selectedId: string | null;
    select: (deviceId: string) => Promise<void>;
  };

  const sections = $derived<DeviceSection[]>([
    {
      label: m('voice.microphone'),
      devices: voiceCallState.audioDevices,
      selectedId: voiceCallState.selectedDeviceId,
      select: (id) => voiceCallState.setAudioDevice(id)
    },
    {
      label: m('voice.speaker'),
      devices: voiceCallState.audioOutputDevices,
      selectedId: voiceCallState.selectedOutputDeviceId,
      select: (id) => voiceCallState.setAudioOutputDevice(id)
    },
    {
      label: m('voice.camera'),
      devices: voiceCallState.videoDevices,
      selectedId: voiceCallState.selectedVideoDeviceId,
      select: (id) => voiceCallState.setVideoDevice(id)
    }
  ]);
</script>

<ContextMenu {anchor} {onclose}>
  {#each sections as section (section.label)}
    <MenuSection ariaLabel={section.label}>
      <div class="px-3 py-1.5 text-xs font-medium text-muted">{section.label}</div>
      {#each section.devices as device (device.deviceId)}
        <MenuItem
          onclick={async () => {
            await section.select(device.deviceId);
            onclose();
          }}
        >
          {#snippet leading()}
            {#if device.deviceId === section.selectedId}
              <span class="iconify icon-[uil--check] text-action"></span>
            {/if}
          {/snippet}
          <span class="block truncate">{device.label || m('voice.unknown_device')}</span>
        </MenuItem>
      {/each}

      {#if section.devices.length === 0}
        <div class="px-3 py-2 text-sm text-muted">{m('voice.no_devices')}</div>
      {/if}
    </MenuSection>
  {/each}
</ContextMenu>
