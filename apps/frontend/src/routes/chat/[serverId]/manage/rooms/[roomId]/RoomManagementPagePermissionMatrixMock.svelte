<script lang="ts">
  let { scrollContents = true }: { scrollContents?: boolean } = $props();
</script>

<div data-testid="permission-matrix" data-scroll-contents={scrollContents}></div>
