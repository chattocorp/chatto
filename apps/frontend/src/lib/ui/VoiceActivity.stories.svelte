<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import VoiceActivity from './VoiceActivity.svelte';

  const { Story } = defineMeta({
    title: 'UI/VoiceActivity',
    component: VoiceActivity,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  /** Speech-like level: bursts of activity with pauses. */
  function speechLevel() {
    const time = performance.now() / 1000;
    return time % 7 < 4.5 ? 0.005 + Math.pow((Math.sin(time * 8) + 1) / 2, 2) * 0.075 : 0;
  }
</script>

<Story name="Behind a button" asChild>
  <div class="flex items-center gap-4">
    <button type="button" class="pill-button relative isolate h-9 rounded-lg px-3">
      <VoiceActivity level={speechLevel} />
      Speaking
    </button>
    <button type="button" class="pill-button relative isolate h-9 rounded-lg px-3">
      <VoiceActivity level={() => 0} />
      Quiet
    </button>
  </div>
</Story>
