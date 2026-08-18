<!--
@component

Renders a LiveKit video track in a thumbnail-sized `<video>` element.
It can optionally include a small avatar overlay in the top-left corner for
identification.

Manages the attach/detach lifecycle imperatively — only detaches/reattaches
when the track reference actually changes, not on every parent re-render.
This prevents flicker from the 60ms audio level polling in VoiceCallPanel.

The explicit width/height attributes tell LiveKit's `adaptiveStream` what
resolution to request for sidebar-width tiles.

**Props:**
- `track` - The LiveKit video Track to display
- `name` - Participant display name (shown as tooltip)
- `user` - User object for the avatar overlay (same shape as UserAvatar's `user` prop)
- `showIdentityOverlay` - Whether to show the avatar overlay
- `fit` - How the video track should fit the tile. Camera thumbnails default to `cover`; screen shares should use `contain` to avoid cropping shared content.
- `fill` - Whether the video should fill its parent's height instead of using thumbnail aspect-ratio sizing.
- `preferFullQuality` - Render a single-layer screen share without adaptive-stream tile sizing.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import type { RemoteVideoTrack, Track } from 'livekit-client';
	import type { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
	import UserAvatar from '$lib/components/UserAvatar.svelte';

	let {
		track,
		name,
		user,
		showIdentityOverlay = true,
		fit = 'cover',
		fill = false,
		preferFullQuality = false
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
		showIdentityOverlay?: boolean;
		fit?: 'cover' | 'contain';
		fill?: boolean;
		preferFullQuality?: boolean;
	} = $props();

	let videoEl = $state<HTMLVideoElement | null>(null);

	// Track what's currently attached to avoid unnecessary detach/reattach cycles.
	// The parent's audio level polling (60ms) triggers $derived recalculations that
	// pass the same Track reference — we must not detach/reattach on those no-ops.
	let attachedTrack: Track | null = null;
	let attachedEl: HTMLVideoElement | null = null;
	let attachedDirectly = false;
	let videoFrameCallbackId: number | null = null;
	let presentationIntervalStartedAt = 0;
	let presentationIntervalStartedFrames = 0;

	function stopPresentationMetrics() {
		if (videoFrameCallbackId !== null && attachedEl) {
			attachedEl.cancelVideoFrameCallback(videoFrameCallbackId);
		}
		videoFrameCallbackId = null;
		presentationIntervalStartedAt = 0;
		presentationIntervalStartedFrames = 0;
	}

	function startPresentationMetrics(el: HTMLVideoElement) {
		if (typeof el.requestVideoFrameCallback !== 'function') return;

		const handleFrame: VideoFrameRequestCallback = (now, metadata) => {
			if (presentationIntervalStartedAt === 0) {
				presentationIntervalStartedAt = now;
				presentationIntervalStartedFrames = metadata.presentedFrames;
			} else if (now - presentationIntervalStartedAt >= 2_000) {
				const elapsedSeconds = (now - presentationIntervalStartedAt) / 1_000;
				console.info('[Chatto] Native screen-share presentation metrics', {
					presentedFps:
						(metadata.presentedFrames - presentationIntervalStartedFrames) / elapsedSeconds,
					presentedFrames: metadata.presentedFrames,
					mediaWidth: metadata.width,
					mediaHeight: metadata.height,
					elementWidth: el.clientWidth,
					elementHeight: el.clientHeight,
					documentVisible: document.visibilityState === 'visible'
				});
				presentationIntervalStartedAt = now;
				presentationIntervalStartedFrames = metadata.presentedFrames;
			}

			videoFrameCallbackId = el.requestVideoFrameCallback(handleFrame);
		};

		videoFrameCallbackId = el.requestVideoFrameCallback(handleFrame);
	}

	function detachCurrentTrack() {
		stopPresentationMetrics();
		if (!attachedTrack || !attachedEl) return;
		if (attachedDirectly) {
			attachedEl.srcObject = null;
		} else {
			attachedTrack.detach(attachedEl);
		}
		attachedDirectly = false;
	}

	$effect(() => {
		const t = track;
		const el = videoEl;

		const direct = preferFullQuality && !!t && 'mediaStreamTrack' in t;
		if (t === attachedTrack && el === attachedEl && direct === attachedDirectly) return;

		detachCurrentTrack();

		attachedTrack = t ?? null;
		attachedEl = el ?? null;
		attachedDirectly = direct;

		if (t && el) {
			if (direct) {
				const remoteTrack = t as RemoteVideoTrack;
				el.srcObject = new MediaStream([remoteTrack.mediaStreamTrack]);
				void el.play().catch(() => undefined);
				startPresentationMetrics(el);
			} else {
				t.attach(el);
			}
		}
	});

	onDestroy(() => {
		detachCurrentTrack();
		attachedTrack = null;
		attachedEl = null;
	});
</script>

<div
	class={[
		'relative block w-full overflow-hidden rounded-md',
		fit === 'contain' ? 'bg-black' : 'bg-surface-emphasized',
		fill ? 'h-full min-h-0' : 'aspect-video'
	]}
>
	<video
		bind:this={videoEl}
		width="640"
		height="360"
		class={['h-full w-full', fit === 'contain' ? 'object-contain' : 'object-cover']}
		title={name}
		autoplay
		playsinline
		muted
	></video>
	{#if showIdentityOverlay}
		<div
			class="absolute top-2 left-2 h-6 w-6 rounded-full shadow-[0_0_0_1.5px_var(--color-surface)]"
		>
			<UserAvatar {user} size="xs" />
		</div>
	{/if}
</div>
