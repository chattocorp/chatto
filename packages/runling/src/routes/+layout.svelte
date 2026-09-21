<script lang="ts">
  import "$lib/theme.css";
  import "@fontsource-variable/plus-jakarta-sans/wght.css";
  import "@fontsource-variable/plus-jakarta-sans/wght-italic.css";
  import type { Snippet } from "svelte";

  let { children }: { children: Snippet } = $props();
</script>

{@render children()}
