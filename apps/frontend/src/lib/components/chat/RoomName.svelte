<!--
@component

A channel room name with its hash or universal-room globe. The name wraps and
isolates user-authored text from the surrounding reading direction.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { roomIcon } from '$lib/render/roomIcon';

  let { name, isUniversal = false }: { name: string; isUniversal?: boolean } = $props();
  const icon = $derived(roomIcon({ isUniversal }));
</script>

<span class="inline-flex min-w-0 items-baseline gap-2">
  <span
    class={['shrink-0 text-muted', isUniversal && 'self-center', icon.className]}
    role={isUniversal ? 'img' : undefined}
    aria-label={isUniversal ? m('room.create.universal_label') : undefined}
    aria-hidden={isUniversal ? undefined : true}>{icon.text}</span
  >
  <bdi class="min-w-0 break-words">{name}</bdi>
</span>
