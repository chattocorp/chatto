<script module lang="ts">
	import { defineMeta } from '@storybook/addon-svelte-csf';
	import VoiceCallPanelStoryHarness from './VoiceCallPanelStoryHarness.svelte';

	const { Story } = defineMeta({
		title: 'Voice/Voice call panel',
		component: VoiceCallPanelStoryHarness,
		tags: ['autodocs'],
		parameters: {
			docs: {
				description: {
					component:
						'Composed call panel layouts rendered with deterministic in-memory call participants.'
				}
			}
		}
	});
</script>

<Story name="Stage with screen share" asChild>
	<div class="flex h-[720px] w-[1080px] overflow-hidden rounded-md border border-border bg-background">
		<VoiceCallPanelStoryHarness layout="stage" scenario="screen" />
	</div>
</Story>

<Story name="Stage with one secondary tile" asChild>
	<div class="flex h-[720px] w-[1080px] overflow-hidden rounded-md border border-border bg-background">
		<VoiceCallPanelStoryHarness layout="stage" scenario="screen-single-secondary" />
	</div>
</Story>

<Story name="Stage camera fallback" asChild>
	<div class="flex h-[720px] w-[1080px] overflow-hidden rounded-md border border-border bg-background">
		<VoiceCallPanelStoryHarness layout="stage" scenario="camera" />
	</div>
</Story>

<Story name="Stage voice fallback" asChild>
	<div class="flex h-[720px] w-[1080px] overflow-hidden rounded-md border border-border bg-background">
		<VoiceCallPanelStoryHarness layout="stage" scenario="voice" />
	</div>
</Story>

<Story name="Sidebar" asChild>
	<div class="flex h-[720px] w-[360px] overflow-hidden rounded-md border border-border bg-background">
		<VoiceCallPanelStoryHarness layout="sidebar" scenario="screen" />
	</div>
</Story>
