<script lang="ts">
  import type { TaskMessage } from "$lib/timeline.ts";
  import type { TimeWindow } from "$lib/timeline-layout.ts";

  let { messages, view, width, onselect }: {
    messages: TaskMessage[];
    view: TimeWindow;
    width: number;
    onselect: (id: string) => void;
  } = $props();

  // Nearby events share a marker so short tasks remain readable and clickable.
  let groups = $derived.by(() => {
    const groups: { left: number; messages: TaskMessage[] }[] = [];
    for (const message of messages) {
      const left = ((message.timestamp - view.start) / view.span) * 100;
      if (left < 0 || left > 100) continue;

      const previous = groups.at(-1);
      if (previous && ((left - previous.left) / 100) * width < 48) {
        previous.messages.push(message);
      } else {
        groups.push({ left, messages: [message] });
      }
    }
    return groups;
  });
</script>

{#each groups as group}
  {@const first = group.messages[0]!}
  <!-- The preceding lane has a 1px bottom border, so the gap midpoint is 0.5px above this lane. -->
  <button
    data-message-marker
    class="group absolute -top-[0.5px] z-3 flex h-7 min-w-7 -translate-x-1/2 -translate-y-1/2 cursor-pointer items-center justify-center px-1 focus-visible:outline-2 focus-visible:outline-primary"
    style:left={`clamp(24px, ${group.left}%, calc(100% - 24px))`}
    aria-label={group.messages.length > 1 ? `${group.messages.length} task messages` : `Message: ${first.from} → ${first.to}. ${first.status}`}
    title={group.messages.length > 1 ? `${group.messages.length} messages — click to inspect` : `${first.status}: ${first.payload.slice(0, 100)}`}
    onclick={() => onselect(first.id)}
  >
    <span class="flex h-4 items-center gap-1 rounded-sm bg-linear-to-b from-yellow-200 to-yellow-300 px-1 text-yellow-950 group-hover:brightness-105">
      {#if group.messages.some(message => message.direction === "input")}
        <span class="icon-[lucide--arrow-down] size-3.5" aria-hidden="true"></span>
      {/if}
      {#if group.messages.some(message => message.direction === "update")}
        <span class="icon-[lucide--arrow-up] size-3.5" aria-hidden="true"></span>
      {/if}
      {#if group.messages.length > 1}<span class="pl-0.5 text-[10px] leading-none font-medium tabular-nums">{group.messages.length}</span>{/if}
    </span>
  </button>
{/each}
