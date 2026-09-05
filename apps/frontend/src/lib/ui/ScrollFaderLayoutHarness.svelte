<script lang="ts">
  import ScrollFader from './ScrollFader.svelte';

  let { intrinsic = false }: { intrinsic?: boolean } = $props();
  let height = $state(60);
  let viewportHeight = $state(200);
  let image = $state<string>();

  export function resizeContent(value: number) {
    height = value;
  }
  export function resizeViewport(value: number) {
    viewportHeight = value;
  }
  export function loadImage() {
    image =
      'data:image/svg+xml,' +
      encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="100" height="400"></svg>');
  }
</script>

<div class="flex w-64 flex-col" style:height={intrinsic ? undefined : `${viewportHeight}px`}>
  <ScrollFader
    top
    bottom
    fill={!intrinsic}
    scrollClass={intrinsic ? 'max-h-52' : ''}
    data-testid="layout-scroll"
  >
    <div class="mt-auto shrink-0" data-testid="layout-content" style:height={`${height}px`}></div>
    {#if image}<img src={image} alt="" class="w-[100px] shrink-0" />{/if}
  </ScrollFader>
</div>
