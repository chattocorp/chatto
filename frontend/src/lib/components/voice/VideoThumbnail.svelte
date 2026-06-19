<!--
@component

Renders a LiveKit video track in a thumbnail-sized `<video>` element with
a small avatar overlay in the top-left corner for identification.

Manages the attach/detach lifecycle imperatively — only detaches/reattaches
when the track reference actually changes, not on every parent re-render.
This prevents flicker from the 60ms audio level polling in VoiceCallPanel.

The explicit width/height attributes tell LiveKit's `adaptiveStream` what
resolution to request (h180 simulcast layer for 120px thumbnails).

**Props:**
- `track` - The LiveKit video Track to display
- `name` - Participant display name (shown as tooltip)
- `user` - User object for the avatar overlay (same shape as UserAvatar's `user` prop)
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import type { Track } from 'livekit-client';
	import type { PresenceStatus } from '$lib/chatTypes';
	import UserAvatar from '$lib/components/UserAvatar.svelte';

	let {
		track,
		name,
		user
	}: {
		track: Track;
		name: string;
		user: {
			id: string;
			login: string;
			displayName: string;
			avatarUrl: string | null;
			presenceStatus: PresenceStatus;
		};
	} = $props();

	let videoEl = $state<HTMLVideoElement | null>(null);

	// Track what's currently attached to avoid unnecessary detach/reattach cycles.
	// The parent's audio level polling (60ms) triggers $derived recalculations that
	// pass the same Track reference — we must not detach/reattach on those no-ops.
	let attachedTrack: Track | null = null;
	let attachedEl: HTMLVideoElement | null = null;

	$effect(() => {
		const t = track;
		const el = videoEl;

		if (t === attachedTrack && el === attachedEl) return;

		if (attachedTrack && attachedEl) {
			attachedTrack.detach(attachedEl);
		}

		if (t && el) {
			t.attach(el);
		}

		attachedTrack = t ?? null;
		attachedEl = el ?? null;
	});

	onDestroy(() => {
		if (attachedTrack && attachedEl) {
			attachedTrack.detach(attachedEl);
			attachedTrack = null;
			attachedEl = null;
		}
	});
</script>

<div class="video-thumbnail">
	<video
		bind:this={videoEl}
		width="120"
		height="68"
		class="rounded object-cover"
		title={name}
		autoplay
		playsinline
		muted
	></video>
	<div class="avatar-badge">
		<UserAvatar {user} size="xs" showPresence={false} />
	</div>
</div>

<style>
	.video-thumbnail {
		position: relative;
		display: inline-flex;
	}

	.avatar-badge {
		position: absolute;
		top: 2px;
		left: 2px;
		width: 1.25rem;
		height: 1.25rem;
		border-radius: 9999px;
		box-shadow: 0 0 0 1.5px var(--color-surface-100);
	}
</style>
