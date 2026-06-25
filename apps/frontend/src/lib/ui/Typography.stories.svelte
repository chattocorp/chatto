<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({
    title: 'Foundations/Typography',
    tags: ['autodocs']
  });
</script>

<Story name="Type scale" asChild>
  <div class="flex max-w-prose flex-col gap-2">
    <p class="text-xs text-muted">text-xs — labels, timestamps</p>
    <p class="text-sm text-muted">text-sm — secondary copy, hints</p>
    <p class="text-base">text-base — body (default)</p>
    <p class="text-lg">text-lg</p>
    <p class="text-xl">text-xl</p>
    <p class="text-2xl">text-2xl</p>
    <p class="text-3xl font-bold">text-3xl bold</p>
  </div>
</Story>

<Story name="Families" asChild>
  <div class="flex flex-col gap-3">
    <div>
      <div class="text-xs uppercase text-muted">font-sans (default)</div>
      <p class="font-sans text-lg">
        IBM Plex Sans Variable — The quick brown fox jumps over the lazy dog. 0123456789
      </p>
    </div>
    <div>
      <div class="text-xs uppercase text-muted">font-mono</div>
      <p class="font-mono text-lg">
        IBM Plex Mono — The quick brown fox jumps over the lazy dog. 0123456789
      </p>
    </div>
  </div>
</Story>

<Story name="Weights & styles" asChild>
  <div class="flex flex-col gap-1">
    <p class="font-light">font-light — Light weight body</p>
    <p>font-normal — Normal body</p>
    <p class="font-medium">font-medium — Medium weight</p>
    <p class="font-semibold">font-semibold — Semibold (used for headings)</p>
    <p class="font-bold">font-bold</p>
    <p class="italic">italic — emphasized text</p>
  </div>
</Story>
