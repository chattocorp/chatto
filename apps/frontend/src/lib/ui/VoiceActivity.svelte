<!-- @component
Decorative audio-level layer. Place inside an isolated, positioned container.
The getter reads a normalized level without updating the surrounding UI.
-->
<script lang="ts">
  import { voiceActivity } from './voiceActivity';
  let { level }: { level: () => number } = $props();
</script>

<canvas
  {@attach voiceActivity(() => level())}
  class="pointer-events-none absolute inset-0 -z-10 h-full w-full rounded-[inherit] text-action"
  aria-hidden="true"
  data-testid="voice-activity"
></canvas>
