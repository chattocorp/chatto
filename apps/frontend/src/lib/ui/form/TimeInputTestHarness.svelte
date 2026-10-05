<script lang="ts">
  import { untrack } from 'svelte';
  import TimeInput from './TimeInput.svelte';

  let { value = '', hour12 }: { value?: string; hour12: boolean } = $props();
  // The harness owns the bound value after mount, so a test can replace it from outside.
  let bound = $state(untrack(() => value));
</script>

<TimeInput id="time" bind:value={bound} {hour12} />
<output aria-label="Bound value">{bound}</output>
<button type="button" onclick={() => (bound = '07:05')}>Set value</button>
