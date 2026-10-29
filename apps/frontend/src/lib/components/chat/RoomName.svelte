<!--
@component

A channel room name with its hash or universal-room globe. The name wraps and
isolates user-authored text from the surrounding reading direction.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';

  let { name, isUniversal = false }: { name: string; isUniversal?: boolean } = $props();
</script>

<span class="inline-flex min-w-0 items-baseline gap-2">
  {#if isUniversal}
    <span
      class="iconify icon-[uil--globe] shrink-0 self-center text-muted"
      role="img"
      aria-label={m('room.create.universal_label')}
    ></span>
  {:else}
    <span class="shrink-0 text-muted" aria-hidden="true">#</span>
  {/if}
  <bdi class="min-w-0 break-words">{name}</bdi>
</span>
