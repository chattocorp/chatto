<!--
@component

Loading skeleton placeholder for link preview cards.
Displays an animated shimmer layout matching the LinkPreviewCard dimensions.
-->
<script lang="ts">
  // No props needed
</script>

<div
  class="flex w-full max-w-md animate-pulse gap-3 rounded-lg border-6 border-surface-100 bg-surface-100 p-3"
>
  <!-- Image skeleton -->
  <div class="bg-outline/20 h-20 w-20 shrink-0 rounded"></div>
  <div class="flex min-w-0 flex-1 flex-col gap-2">
    <!-- Site name skeleton -->
    <div class="bg-outline/20 h-3 w-16 rounded"></div>
    <!-- Title skeleton -->
    <div class="bg-outline/20 h-4 w-3/4 rounded"></div>
    <!-- Description skeleton -->
    <div class="bg-outline/20 h-3 w-full rounded"></div>
    <div class="bg-outline/20 h-3 w-2/3 rounded"></div>
  </div>
</div>
