<script lang="ts">
  import {
    useMayHaveMissedMessagesCallback,
    type MayHaveMissedMessagesReason
  } from './useMayHaveMissedMessagesCallback.svelte';

  let {
    onSignal
  }: {
    onSignal: (reason: MayHaveMissedMessagesReason) => boolean | void | Promise<boolean | void>;
  } = $props();

  useMayHaveMissedMessagesCallback((reason) => onSignal(reason));
</script>

<div data-testid="maybe-missed-harness"></div>
