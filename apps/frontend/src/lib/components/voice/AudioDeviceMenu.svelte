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
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import * as m from '$lib/i18n/messages';

  const voiceCallState = serverRegistry.getStore(getActiveServer()).voiceCall;
  import ContextMenu from '$lib/ui/ContextMenu.svelte';

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
      label: m['voice.microphone'](),
      devices: voiceCallState.audioDevices,
      selectedId: voiceCallState.selectedDeviceId,
      select: (id) => voiceCallState.setAudioDevice(id)
    },
    {
      label: m['voice.speaker'](),
      devices: voiceCallState.audioOutputDevices,
      selectedId: voiceCallState.selectedOutputDeviceId,
      select: (id) => voiceCallState.setAudioOutputDevice(id)
    },
    {
      label: m['voice.camera'](),
      devices: voiceCallState.videoDevices,
      selectedId: voiceCallState.selectedVideoDeviceId,
      select: (id) => voiceCallState.setVideoDevice(id)
    }
  ]);
</script>

<ContextMenu {anchor} {onclose}>
  {#each sections as section (section.label)}
    <div class="menu-section">
      <div class="px-3 py-1.5 text-xs font-medium text-muted">{section.label}</div>
      <nav class="sidebar-nav">
        {#each section.devices as device (device.deviceId)}
          <button
            class="sidebar-item"
            role="menuitem"
            onclick={async () => {
              await section.select(device.deviceId);
              onclose();
            }}
          >
            {#if device.deviceId === section.selectedId}
              <span class="sidebar-icon iconify text-action uil--check"></span>
            {:else}
              <span class="sidebar-icon"></span>
            {/if}
            <span class="truncate">{device.label || m['voice.unknown_device']()}</span>
          </button>
        {/each}

        {#if section.devices.length === 0}
          <div class="px-3 py-2 text-sm text-muted">{m['voice.no_devices']()}</div>
        {/if}
      </nav>
    </div>
  {/each}
</ContextMenu>
