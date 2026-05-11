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

	const getInstanceId = getActiveServer();
	const voiceCallState = serverRegistry.getStore(getInstanceId()).voiceCall;
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
		select: (deviceId: string) => void;
	};

	const sections: DeviceSection[] = [
		{
			label: 'Microphone',
			devices: voiceCallState.audioDevices,
			selectedId: voiceCallState.selectedDeviceId,
			select: (id) => voiceCallState.setAudioDevice(id)
		},
		{
			label: 'Speaker',
			devices: voiceCallState.audioOutputDevices,
			selectedId: voiceCallState.selectedOutputDeviceId,
			select: (id) => voiceCallState.setAudioOutputDevice(id)
		},
		{
			label: 'Camera',
			devices: voiceCallState.videoDevices,
			selectedId: voiceCallState.selectedVideoDeviceId,
			select: (id) => voiceCallState.setVideoDevice(id)
		}
	];
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
						onclick={() => {
							section.select(device.deviceId);
							onclose();
						}}
					>
						{#if device.deviceId === section.selectedId}
							<span class="sidebar-icon iconify uil--check text-accent"></span>
						{:else}
							<span class="sidebar-icon"></span>
						{/if}
						<span class="truncate">{device.label || 'Unknown device'}</span>
					</button>
				{/each}

				{#if section.devices.length === 0}
					<div class="px-3 py-2 text-sm text-muted">No devices found</div>
				{/if}
			</nav>
		</div>
	{/each}
</ContextMenu>
