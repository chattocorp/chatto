<script lang="ts">
  import { ansiTokens } from "$lib/ansi.ts";
  let { text }: { text: string } = $props();
  let tokens = $derived(ansiTokens(text));
</script>

{#each tokens as token}<span style={token.style}>{token.text}</span>{/each}
