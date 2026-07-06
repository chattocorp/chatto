<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SkeletonImg from './SkeletonImg.svelte';

  const { Story } = defineMeta({
    title: 'UI/SkeletonImg',
    component: SkeletonImg,
    tags: ['autodocs']
  });
</script>

<Story name="Loading (broken src)" asChild>
  <p class="mb-3 text-sm text-muted">
    The src below 404s on purpose so the shimmer keeps animating. In real use,
    the shimmer disappears as soon as <code>onload</code> fires.
  </p>
  <SkeletonImg
    src="/storybook/never-resolves-{Math.random()}.png"
    alt=""
    class="h-32 w-32 rounded-md"
  />
</Story>

<Story name="Skeleton utility (raw)" asChild>
  <p class="mb-3 text-sm text-muted">
    The same shimmer treatment applied directly via the <code>.skeleton</code>
    utility on arbitrary elements.
  </p>
  <div class="flex flex-col gap-2">
    <div class="skeleton h-5 w-64 rounded-md"></div>
    <div class="skeleton h-5 w-48 rounded-md"></div>
    <div class="skeleton h-5 w-56 rounded-md"></div>
  </div>
</Story>
