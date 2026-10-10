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

<Story name="Playable media controls" asChild>
  <p class="mb-2">Right-click a video to open its user menu and picture-in-picture control.</p>
  <div
    class="flex h-[720px] w-[1080px] flex-col overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="stage" scenario="screen" playableMedia />
  </div>
</Story>

<Story name="Playable sidebar media controls" asChild>
  <p class="mb-2">Right-click a camera or screen share to open its user menu.</p>
  <div
    class="flex h-[720px] w-[360px] flex-col overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="sidebar" scenario="screen" playableMedia />
  </div>
</Story>

<Story name="Narrow card overflow" asChild>
  <div
    class="flex h-[720px] w-[500px] flex-col overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="sidebar" scenario="screen" playableMedia />
  </div>
</Story>

<Story name="Wide card controls" asChild>
  <div
    class="flex h-[720px] w-[360px] flex-col overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="sidebar" scenario="screen" playableMedia />
  </div>
</Story>

<Story name="Live voice glow" asChild>
  <div class="flex h-[600px] w-80 overflow-hidden rounded-md border border-border bg-background">
    <VoiceCallPanelStoryHarness layout="sidebar" scenario="screen" animateVoice />
  </div>
</Story>

<Story name="Stage with screen share" asChild>
  <div
    class="flex h-[720px] w-[1080px] overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="stage" scenario="screen" />
  </div>
</Story>

<Story name="Stage with one secondary tile" asChild>
  <div
    class="flex h-[720px] w-[1080px] overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="stage" scenario="screen-single-secondary" />
  </div>
</Story>

<Story name="Stage camera fallback" asChild>
  <div
    class="flex h-[720px] w-[1080px] overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="stage" scenario="camera" />
  </div>
</Story>

<Story name="Stage active speaker" asChild>
  <div
    class="flex h-[720px] w-[1080px] overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="stage" scenario="camera" activeSpeaker="bob" />
  </div>
</Story>

<Story name="Stage voice grid" asChild>
  <div
    class="flex h-[720px] w-[1080px] overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="stage" scenario="voice" />
  </div>
</Story>

<Story name="Sidebar" asChild>
  <div
    class="flex h-[720px] w-[360px] overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="sidebar" scenario="screen" />
  </div>
</Story>

<Story name="Start call" asChild>
  <div class="flex h-[720px] w-[256px] overflow-hidden rounded-lg bg-background">
    <VoiceCallPanelStoryHarness layout="sidebar" scenario="idle" />
  </div>
</Story>

<Story name="Sidebar voice controls" asChild>
  <div
    class="flex h-[720px] w-[280px] overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="sidebar" scenario="voice" />
  </div>
</Story>

<Story name="Screen share with equal voice columns" asChild>
  <div
    class="flex h-[720px] w-[500px] overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="sidebar" scenario="screen-voice" />
  </div>
</Story>

<Story name="Sidebar muted microphone" asChild>
  <div
    class="flex h-[720px] w-[280px] overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="sidebar" scenario="voice" initiallyMuted />
  </div>
</Story>

<Story name="Sidebar with many cameras" asChild>
  <div
    class="flex h-[600px] w-[360px] overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="sidebar" scenario="screen" extraParticipants={16} />
  </div>
</Story>

<Story name="Sidebar speaker following" asChild>
  <div
    class="flex h-[600px] w-[280px] overflow-hidden rounded-md border border-border bg-background"
  >
    <VoiceCallPanelStoryHarness layout="sidebar" scenario="camera" activeSpeaker="bob" />
  </div>
</Story>
