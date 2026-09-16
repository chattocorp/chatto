<script module lang="ts">
	import { defineMeta } from '@storybook/addon-svelte-csf';
	import ServerBanner from './ServerBanner.svelte';

	const artwork = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630"><rect width="1200" height="630" fill="#28566b"/><circle cx="920" cy="130" r="240" fill="#b8c9bc"/><path d="M0 530 320 160 720 630H0" fill="#163443"/><path d="m400 630 440-350 360 350" fill="#467b85"/></svg>')}`;
	const { Story } = defineMeta({
		title: 'Components/Server banner',
		component: ServerBanner,
		tags: ['autodocs']
	});
</script>

<Story name="Sidebar artwork" asChild>
	<div class="w-64 border-x border-border bg-background">
		<div class="border-b border-border px-4 py-3 font-semibold">Design studio</div>
		<ServerBanner url={artwork} />
		<nav class="sidebar-nav p-2" aria-label="Server navigation">
			<button type="button" class="sidebar-item">
				<span class="iconify sidebar-icon icon-[uil--estate]" aria-hidden="true"></span>
				Overview
			</button>
			<button type="button" class="sidebar-item">
				<span class="iconify sidebar-icon icon-[uil--comment-alt]" aria-hidden="true"></span>
				General
			</button>
		</nav>
	</div>
</Story>
