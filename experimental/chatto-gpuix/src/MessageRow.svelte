<script lang="ts">
  import type { DemoMessage } from './demo.ts';
  import MessageFrame from './ui/MessageFrame.svelte';
  import { markdownTheme } from './ui/theme.ts';
  let { message }: { message: DemoMessage } = $props();
</script>

<MessageFrame author={message.author} time={message.time}>
  {#if message.format === 'markdown'}
    <markdown source={message.body} theme={markdownTheme} class="body"></markdown>
  {:else}<text class="body plain">{message.body}</text>{/if}
</MessageFrame>

<style>
  .body {
    color: var(--color-text);
    font-size: 14px;
    user-select: text;
  }
  .plain {
    white-space: normal;
  }
</style>
