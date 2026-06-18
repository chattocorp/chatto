<script lang="ts">
	import { tick, onMount } from 'svelte';
	import type { VideoProcessingStatus } from '$lib/gql/graphql';
	import { fullscreenVideo } from '$lib/state/globals.svelte';

	import 'vidstack/player/styles/default/theme.css';
	import 'vidstack/player/styles/default/layouts/video.css';

	// Vidstack ships empty server stubs under the "default" export condition;
	// static imports in SvelteKit resolve those stubs during SSR and never
	// re-run on the client. We must dynamically import on mount and wait for
	// registration to complete before rendering the custom elements.
	let elementsReady = $state(false);

	onMount(async () => {
		await Promise.all([
			import('vidstack/player'),
			import('vidstack/player/layouts'),
			import('vidstack/player/ui')
		]);
		elementsReady = true;
	});

	type Variant = {
		url: string;
		quality: string;
		width: number;
		height: number;
		size: number;
	};

	let {
		status,
		variants = [],
		thumbnailUrl = null,
		width = null,
		height = null,
		reasonCode = null,
		filename,
		autoLoop = false,
		onMediaError
	}: {
		status: VideoProcessingStatus;
		variants?: Variant[];
		thumbnailUrl?: string | null;
		width?: number | null;
		height?: number | null;
		reasonCode?: string | null;
		filename: string;
		autoLoop?: boolean;
		onMediaError?: () => void;
	} = $props();

	const MAX_WIDTH = 480;
	const MAX_HEIGHT = 320;

	const displaySize = $derived.by(() => {
		const w = width || 480;
		const h = height || 270;
		const scale = Math.min(MAX_WIDTH / w, MAX_HEIGHT / h, 1);
		return {
			width: Math.round(w * scale),
			height: Math.round(h * scale)
		};
	});

	// Pick the best variant (highest quality available)
	const selectedVariant = $derived(
		variants.length > 0
			? variants.reduce((best, v) => (v.height > best.height ? v : best), variants[0])
			: null
	);

	// Vidstack auto-detects media type from URL extensions, but our attachment
	// URLs have no extension (/assets/attachments/...). We must provide an
	// explicit type so Vidstack recognizes it as video/mp4.
	const videoSrc = $derived(
		selectedVariant ? { src: selectedVariant.url, type: 'video/mp4' } : undefined
	);

	const failureMessage = $derived.by(() => {
		switch (reasonCode) {
			case 'original_missing':
				return 'Video is unavailable because the original upload is missing.';
			case 'processing_failed':
				return 'Video processing failed. Please try uploading again.';
			default:
				return null;
		}
	});

	let playerEl = $state<HTMLElement | null>(null);

	// Intercept Vidstack's fullscreen request — the <media-player> lives inside
	// virtua's virtualized list, so native fullscreen would cause virtua to
	// unmount the DOM node. Instead, open our CSS overlay outside the list.
	$effect(() => {
		if (!playerEl) return;

		function handleFullscreenRequest(e: Event) {
			e.preventDefault();
			if (!selectedVariant) return;

			const video = playerEl?.querySelector('video');
			if (video) video.pause();

			fullscreenVideo.open(
				selectedVariant.url,
				thumbnailUrl ?? null,
				video?.currentTime ?? 0
			);

			// Request native fullscreen on the overlay after Svelte renders it.
			// tick() preserves the user activation from this click event.
			tick().then(() => {
				document
					.querySelector('.fullscreen-overlay')
					?.requestFullscreen()
					.catch(() => {});
			});
		}

		// Use capture phase so we intercept before Vidstack's internal handler.
		playerEl.addEventListener('media-enter-fullscreen-request', handleFullscreenRequest, true);
		return () => {
			playerEl?.removeEventListener('media-enter-fullscreen-request', handleFullscreenRequest, true);
		};
	});

</script>

{#if status === 'COMPLETED' && selectedVariant && autoLoop}
	<!-- Converted GIFs use a native <video> for reliable autoplay + loop behavior. -->
	<div
		class="embed-frame"
		style="width: {displaySize.width}px; max-width: 100%;"
	>
		<video
			autoplay
			loop
			muted
			playsinline
			data-autoloop
			onerror={onMediaError}
			style="aspect-ratio: {displaySize.width} / {displaySize.height}; width: 100%;"
		>
			<source src={selectedVariant.url} type="video/mp4" onerror={onMediaError} />
		</video>
	</div>
{:else if status === 'COMPLETED' && selectedVariant && elementsReady}
	<div
		class="embed-frame"
		style="width: {displaySize.width}px; max-width: 100%;"
	>
		<media-player
			bind:this={playerEl}
			src={videoSrc}
			playsinline
			onerror={onMediaError}
			style="aspect-ratio: {displaySize.width} / {displaySize.height};"
		>
			<media-provider>
				{#if thumbnailUrl}
					<media-poster
						class="vds-poster"
						src={thumbnailUrl}
						alt={filename}
						onerror={onMediaError}
					></media-poster>
				{/if}
			</media-provider>
			<media-video-layout></media-video-layout>
		</media-player>

	</div>
{:else if status === 'PENDING' || status === 'PROCESSING'}
	<div
		class="flex items-center gap-3 px-4 py-3 embed-frame"
		style="width: {displaySize.width}px; max-width: 100%;"
	>
		<div class="h-5 w-5 animate-spin rounded-full border-2 border-muted border-t-transparent"></div>
		<div class="text-sm text-muted">
			{status === 'PENDING' ? 'Video queued for processing...' : 'Processing video...'}
		</div>
	</div>
{:else if status === 'FAILED'}
	<div
		class="flex items-center gap-3 px-4 py-3 embed-frame"
		style="width: {displaySize.width}px; max-width: 100%;"
	>
		<span class="iconify uil--exclamation-triangle text-lg text-red-400"></span>
		<div class="text-sm text-muted">
			Video processing failed
			{#if failureMessage}
				<span class="block text-xs text-muted/70">{failureMessage}</span>
			{/if}
		</div>
	</div>
{:else}
	<div class="flex items-center gap-2 px-3 py-2 embed-frame">
		<span class="iconify uil--video text-lg text-muted"></span>
		<span class="text-sm">{filename}</span>
	</div>
{/if}

<style>
	/* Hide menus from Vidstack's default layout — not useful for embedded chat videos. */
	:global(media-player .vds-settings-menu),
	:global(media-player .vds-chapters-menu) {
		display: none !important;
	}
</style>
