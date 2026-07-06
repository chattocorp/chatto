<script lang="ts">
  import { useMayHaveMissedMessagesCallback } from './useMayHaveMissedMessagesCallback.svelte';
  import type { ResumeSignal } from './resumeCoordinator.svelte';

  let {
    onSignal
  }: {
    onSignal: (signal: ResumeSignal) => boolean | void | Promise<boolean | void>;
  } = $props();

  useMayHaveMissedMessagesCallback((signal) => onSignal(signal));
</script>

<div data-testid="maybe-missed-harness"></div>
